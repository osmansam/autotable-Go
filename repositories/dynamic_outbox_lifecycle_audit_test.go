package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestOutboxClaimOwnershipLifecycle(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	defer mt.Close()
	event := models.DynamicOutboxEvent{ID: primitive.NewObjectID(), Attempts: 2, MaxAttempts: 3}
	mt.Run("renewal matches current attempt and processing status", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}))
		until := time.Date(2026, 10, 18, 10, 20, 30, 0, time.UTC)
		owned, err := repository.RenewOutboxEventLease(context.Background(), event, until)
		if err != nil || !owned {
			mt.Fatalf("renew = %v, %v", owned, err)
		}
		update := outboxAuditUpdate(mt)
		assertOutboxAuditClaimFilter(mt, update.Lookup("q").Document(), event)
		set := update.Lookup("u").Document().Lookup("$set").Document()
		if set.Lookup("nextAttemptAt").DateTime() != until.UnixMilli() {
			mt.Fatalf("renew deadline = %v", set)
		}
	})
	for _, operation := range []string{"done", "failed", "renew"} {
		mt.Run("stale "+operation+" reports lost ownership", func(mt *mtest.T) {
			repository := mockRepository(mt.Coll)
			mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 0}, bson.E{Key: "nModified", Value: 0}))
			var owned bool
			var err error
			switch operation {
			case "done":
				owned, err = repository.MarkClaimedOutboxEventDone(context.Background(), event)
			case "failed":
				owned, err = repository.MarkClaimedOutboxEventFailed(context.Background(), event, "failed", time.Second)
			case "renew":
				owned, err = repository.RenewOutboxEventLease(context.Background(), event, time.Now().Add(time.Second))
			}
			if err != nil || owned {
				mt.Fatalf("stale %s = %v, %v", operation, owned, err)
			}
			assertOutboxAuditClaimFilter(mt, outboxAuditUpdate(mt).Lookup("q").Document(), event)
		})
	}
	for _, attempt := range []int{2, 3} {
		mt.Run("failure state at attempt "+string(rune('0'+attempt)), func(mt *mtest.T) {
			repository := mockRepository(mt.Coll)
			claimed := event
			claimed.Attempts = attempt
			mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}))
			before := time.Now()
			owned, err := repository.MarkClaimedOutboxEventFailed(context.Background(), claimed, "upstream failure", 9*time.Second)
			if err != nil || !owned {
				mt.Fatalf("fail = %v, %v", owned, err)
			}
			command := outboxAuditUpdate(mt)
			assertOutboxAuditClaimFilter(mt, command.Lookup("q").Document(), claimed)
			update := command.Lookup("u").Document()
			set := update.Lookup("$set").Document()
			want := models.DynamicOutboxStatusPending
			if attempt == 3 {
				want = models.DynamicOutboxStatusFailed
			}
			if set.Lookup("status").StringValue() != want || set.Lookup("lastError").StringValue() != "upstream failure" {
				mt.Fatalf("failure update = %v", update)
			}
			next := time.UnixMilli(set.Lookup("nextAttemptAt").DateTime())
			if next.Before(before.Add(9*time.Second).Truncate(time.Millisecond)) || next.After(time.Now().Add(9*time.Second)) {
				mt.Fatalf("retry deadline = %v", next)
			}
			_, expiryErr := set.LookupErr("expireAt")
			if (attempt == 3) != (expiryErr == nil) {
				mt.Fatalf("terminal retention = %v", update)
			}
			if attempt < 3 && update.Lookup("$unset").Document().Lookup("expireAt").StringValue() != "" {
				mt.Fatalf("retry must clear expiry: %v", update)
			}
		})
	}
	mt.Run("expired final claim is deadlettered", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}))
		now := time.Date(2026, 10, 18, 10, 20, 30, 0, time.UTC)
		if err := repository.RecoverExhaustedOutboxEvents(context.Background(), now); err != nil {
			mt.Fatal(err)
		}
		update := outboxAuditUpdate(mt)
		query := update.Lookup("q").Document()
		if query.Lookup("nextAttemptAt").Document().Lookup("$lte").DateTime() != now.UnixMilli() {
			mt.Fatalf("recovery must exclude live leases: %v", query)
		}
		expr := query.Lookup("$expr").Document().Lookup("$gte").Array()
		values, _ := expr.Values()
		if len(values) != 2 || values[0].StringValue() != "$attempts" || values[1].StringValue() != "$maxAttempts" {
			mt.Fatalf("recovery must require exhausted attempts: %v", query)
		}
		set := update.Lookup("u").Document().Lookup("$set").Document()
		if set.Lookup("status").StringValue() != models.DynamicOutboxStatusFailed || set.Lookup("expireAt").Type != bson.TypeDateTime {
			mt.Fatalf("recovery = %v", set)
		}
	})
}

func outboxAuditUpdate(mt *mtest.T) bson.Raw {
	mt.Helper()
	command := mt.GetStartedEvent().Command
	updates, err := command.Lookup("updates").Array().Values()
	if err != nil || len(updates) != 1 {
		mt.Fatalf("updates = %v, %v", command, err)
	}
	return updates[0].Document()
}

func assertOutboxAuditClaimFilter(mt *mtest.T, filter bson.Raw, event models.DynamicOutboxEvent) {
	mt.Helper()
	if filter.Lookup("_id").ObjectID() != event.ID || filter.Lookup("attempts").Int32() != int32(event.Attempts) || filter.Lookup("status").StringValue() != models.DynamicOutboxStatusProcessing {
		mt.Fatalf("claim ownership filter = %v", filter)
	}
}

func TestOutboxIdempotentInsertScopesAndPropagatesFailures(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	defer mt.Close()
	event := models.DynamicOutboxEvent{ID: primitive.NewObjectID(), TenantID: "tenant", ProjectID: "project", SchemaName: "orders", Payload: models.DynamicOutboxPayload{IdempotencyKey: "tenant:project:orders:notify:step:key"}}
	mt.Run("deduplicates through insert-only upsert", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 0}))
		if _, err := repository.InsertOutboxEvent(context.Background(), event); err != nil {
			mt.Fatal(err)
		}
		update := outboxAuditUpdate(mt)
		if !update.Lookup("upsert").Boolean() || update.Lookup("q").Document().Lookup("payload.idempotencyKey").StringValue() != event.Payload.IdempotencyKey {
			mt.Fatalf("dedupe query = %v", update)
		}
		value := update.Lookup("u").Document().Lookup("$setOnInsert").Document()
		if value.Lookup("_id").ObjectID() != event.ID || value.Lookup("tenantId").StringValue() != "tenant" || value.Lookup("projectId").StringValue() != "project" {
			mt.Fatalf("scoped snapshot = %v", value)
		}
		if _, err := update.Lookup("u").Document().LookupErr("$set"); err == nil {
			mt.Fatal("duplicate enqueue must not overwrite existing attempt state")
		}
	})
	mt.Run("concurrent unique key race succeeds", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		mt.AddMockResponses(mtest.CreateWriteErrorsResponse(mtest.WriteError{Index: 0, Code: 11000, Message: "duplicate idempotency key"}))
		if _, err := repository.InsertOutboxEvent(context.Background(), event); err != nil {
			mt.Fatalf("concurrent enqueue = %v", err)
		}
	})
	mt.Run("storage failure is not deduplicated", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		mt.AddMockResponses(mtest.CreateCommandErrorResponse(mtest.CommandError{Code: 13, Message: "unauthorized"}))
		if _, err := repository.InsertOutboxEvent(context.Background(), event); err == nil {
			mt.Fatal("storage failure was swallowed")
		}
	})
}

func TestOutboxClaimUsesDueLeaseAndAttemptBudget(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	defer mt.Close()
	mt.Run("claim query and lease", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		now := time.Date(2026, 10, 18, 10, 20, 30, 0, time.UTC)
		id := primitive.NewObjectID()
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "value", Value: bson.D{{Key: "_id", Value: id}, {Key: "status", Value: models.DynamicOutboxStatusProcessing}, {Key: "attempts", Value: 2}, {Key: "maxAttempts", Value: 3}}}))
		got, err := repository.ClaimPendingOutboxEvent(context.Background(), now)
		if err != nil || got.ID != id || got.Attempts != 2 {
			mt.Fatalf("claimed = %#v, %v", got, err)
		}
		command := mt.GetStartedEvent().Command
		query := command.Lookup("query").Document()
		if query.Lookup("nextAttemptAt").Document().Lookup("$lte").DateTime() != now.UnixMilli() {
			mt.Fatalf("not-yet-due events can be claimed: %v", query)
		}
		statuses, _ := query.Lookup("status").Document().Lookup("$in").Array().Values()
		if len(statuses) != 2 || statuses[0].StringValue() != models.DynamicOutboxStatusPending || statuses[1].StringValue() != models.DynamicOutboxStatusProcessing {
			mt.Fatalf("claimable statuses = %v", query)
		}
		budget, _ := query.Lookup("$expr").Document().Lookup("$lt").Array().Values()
		if len(budget) != 2 || budget[0].StringValue() != "$attempts" || budget[1].StringValue() != "$maxAttempts" {
			mt.Fatalf("attempt budget = %v", query)
		}
		update := command.Lookup("update").Document()
		if update.Lookup("$inc").Document().Lookup("attempts").Int32() != 1 || update.Lookup("$set").Document().Lookup("nextAttemptAt").DateTime() != now.Add(30*time.Second).UnixMilli() {
			mt.Fatalf("claim attempt/lease = %v", update)
		}
		if !command.Lookup("new").Boolean() || command.Lookup("sort").Document().Lookup("createdAt").Int32() != 1 {
			mt.Fatalf("claim must return new attempt and prefer oldest event: %v", command)
		}
	})
	mt.Run("completed claim has terminal state and retention", func(mt *mtest.T) {
		repository := mockRepository(mt.Coll)
		event := models.DynamicOutboxEvent{ID: primitive.NewObjectID(), Attempts: 2}
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}))
		if owned, err := repository.MarkClaimedOutboxEventDone(context.Background(), event); err != nil || !owned {
			mt.Fatalf("done = %v, %v", owned, err)
		}
		update := outboxAuditUpdate(mt)
		assertOutboxAuditClaimFilter(mt, update.Lookup("q").Document(), event)
		value := update.Lookup("u").Document()
		set := value.Lookup("$set").Document()
		if set.Lookup("status").StringValue() != models.DynamicOutboxStatusDone || set.Lookup("processedAt").Type != bson.TypeDateTime || set.Lookup("expireAt").DateTime() <= set.Lookup("processedAt").DateTime() {
			mt.Fatalf("completed state = %v", set)
		}
		if _, err := value.Lookup("$unset").Document().LookupErr("lastError"); err != nil {
			mt.Fatalf("completion retains failure: %v", value)
		}
	})
}

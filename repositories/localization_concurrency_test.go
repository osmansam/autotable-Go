package repositories

import (
	"context"
	"errors"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
	"testing"
	"time"
)

func TestLocalizationContentDoesNotTreatExplicitSessionAsTransaction(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("explicit session has no transaction", func(mt *mtest.T) {
		r := NewLocalizationRepositoryWithCollections(mt.Coll, mt.Coll, mt.Coll)
		session, err := mt.Client.StartSession()
		if err != nil {
			mt.Fatal(err)
		}
		defer session.EndSession(context.Background())
		job := models.TranslationJob{ID: primitive.NewObjectID(), LeaseOwner: "worker", LeaseToken: "claim"}
		ctx := mongo.NewSessionContext(WithLocalizationJobClaim(context.Background(), job), session)
		_, err = r.UpsertGeneratedTranslation(ctx, models.TranslationEntry{TranslationKey: "title"})
		if err == nil || len(mt.GetAllStartedEvents()) != 0 {
			mt.Fatalf("unowned transaction attempted content write: err=%v events=%d", err, len(mt.GetAllStartedEvents()))
		}
	})
}

func TestLocalizationContentWriteUsesAtomicClaimFence(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("claim CAS and content share transaction", func(mt *mtest.T) {
		r := NewLocalizationRepositoryWithCollections(mt.DB.Collection("translations"), mt.DB.Collection("preferences"), mt.DB.Collection("jobs"))
		job := models.TranslationJob{ID: primitive.NewObjectID(), LeaseOwner: "worker", LeaseToken: "token"}
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}), mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}), mtest.CreateSuccessResponse())
		stored, err := r.UpsertGeneratedTranslation(WithLocalizationJobClaim(context.Background(), job), models.TranslationEntry{TranslationKey: "title"})
		if err != nil || !stored {
			mt.Fatalf("store=%v err=%v", stored, err)
		}
		events := mt.GetAllStartedEvents()
		if len(events) != 3 || events[0].CommandName != "update" || events[1].CommandName != "update" || events[2].CommandName != "commitTransaction" {
			mt.Fatalf("claim and content were not committed together: %#v", events)
		}
		for _, event := range events[:2] {
			if event.Command.Lookup("autocommit").Type != bson.TypeBoolean || event.Command.Lookup("autocommit").Boolean() {
				mt.Fatalf("write outside transaction: %s", event.Command)
			}
		}
		filter := events[0].Command.Lookup("updates").Array().Index(0).Value().Document().Lookup("q").Document()
		if filter.Lookup("leaseToken").StringValue() != "token" {
			mt.Fatal("claim CAS omitted token")
		}
	})
}

func TestReconciliationPreservesTranslationContentAtomically(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("stale snapshot", func(mt *mtest.T) {
		r := NewLocalizationRepositoryWithCollections(mt.Coll, mt.Coll, mt.Coll)
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}))
		if err := r.ApplyReconciledEntries(context.Background(), []models.TranslationEntry{{TenantID: primitive.NewObjectID(), ProjectID: primitive.NewObjectID(), Locale: "tr", TranslationKey: "title", SourceHash: "new", TranslatedText: "stale", Origin: models.TranslationOriginAI}}); err != nil {
			mt.Fatal(err)
		}
		update := mt.GetStartedEvent().Command.Lookup("updates").Array().Index(0).Value().Document().Lookup("u")
		if update.Type != bson.TypeArray {
			mt.Fatal("reconciliation needs an atomic update that preserves current content")
		}
		set := update.Array().Index(0).Value().Document().Lookup("$set").Document()
		if set.Lookup("translatedText").Type == bson.TypeString || set.Lookup("origin").Type == bson.TypeString {
			mt.Fatal("stale text/origin is written unconditionally")
		}
	})
}

func TestLocalizationProgressRejectsLostLease(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("stale completion", func(mt *mtest.T) {
		r := NewLocalizationRepositoryWithCollections(mt.Coll, mt.Coll, mt.Coll)
		job := models.TranslationJob{ID: primitive.NewObjectID(), LeaseOwner: "worker", LeaseToken: "claim"}
		ctx := WithLocalizationJobClaim(context.Background(), job)
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 0}, bson.E{Key: "nModified", Value: 0}))
		if err := r.CompleteJob(ctx, job.ID, 1, 0, 0, nil); !errors.Is(err, ErrLocalizationLeaseLost) {
			mt.Fatalf("stale completion error=%v", err)
		}
		filter := mt.GetStartedEvent().Command.Lookup("updates").Array().Index(0).Value().Document().Lookup("q").Document()
		if filter.Lookup("leaseOwner").StringValue() != "worker" || filter.Lookup("leaseToken").StringValue() != "claim" || filter.Lookup("leaseExpiresAt").Type == 0 {
			mt.Fatalf("unfenced job update: %s", filter)
		}
	})
	mt.Run("unclaimed", func(mt *mtest.T) {
		r := NewLocalizationRepositoryWithCollections(mt.Coll, mt.Coll, mt.Coll)
		if err := r.UpdateJobProgress(context.Background(), primitive.NewObjectID(), 1, 0, 0, nil); !errors.Is(err, ErrLocalizationLeaseLost) {
			mt.Fatalf("unclaimed progress error=%v", err)
		}
		if len(mt.GetAllStartedEvents()) != 0 {
			mt.Fatal("unclaimed worker wrote job progress")
		}
	})
	mt.Run("renewal", func(mt *mtest.T) {
		r := NewLocalizationRepositoryWithCollections(mt.Coll, mt.Coll, mt.Coll)
		job := models.TranslationJob{ID: primitive.NewObjectID(), LeaseOwner: "worker", LeaseToken: "claim"}
		mt.AddMockResponses(mtest.CreateSuccessResponse(bson.E{Key: "n", Value: 1}, bson.E{Key: "nModified", Value: 1}))
		owned, err := r.RenewJobLease(context.Background(), &job, time.Now(), time.Minute)
		if err != nil || !owned {
			mt.Fatalf("renewal=%v %v", owned, err)
		}
	})
}

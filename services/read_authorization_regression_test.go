package services

import (
	"bytes"
	"context"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/osmansam/autotableGo/cache"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/requests"
	"github.com/osmansam/autotableGo/utils"
	"github.com/xuri/excelize/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
	"strings"
	"testing"
	"time"
)

func TestExportProjectsObjectWhenEveryChildIsRestricted(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("member nested object", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".tenant_t_project_p_records", mtest.FirstBatch, bson.D{{Key: "profile", Value: bson.D{{Key: "private", Value: "restricted-marker"}}}}))
		service := &DynamicService{repository: repositories.NewDynamicRepositoryWithDatabase(mt.DB)}
		result, err := service.ExportDynamicItems(context.Background(), ExportDynamicItemsInput{
			TenantID: "t", ProjectID: "p", AccessIdentityInput: AccessIdentityInput{UserRole: "member"},
			Container: &models.ContainerModel{SchemaName: "records", Fields: []models.Field{{Name: "profile", Type: "object", Children: []models.Field{{Name: "private", Type: "string", IsAuthorized: true, AuthorizeRole: []string{"admin"}}}}}},
			Request:   requests.ExportRequest{SchemaName: "records"},
		})
		if err != nil {
			mt.Fatal(err)
		}
		workbook, err := excelize.OpenReader(bytes.NewReader(result.Content))
		if err != nil {
			mt.Fatal(err)
		}
		defer workbook.Close()
		rows, err := workbook.GetRows("Sheet1")
		if err != nil {
			mt.Fatal(err)
		}
		for _, row := range rows {
			for _, cell := range row {
				if strings.Contains(cell, "restricted-marker") {
					mt.Fatalf("nested restricted field exported: %#v", rows)
				}
			}
		}
	})
}

func TestExportOmitsRestrictedAndHashedColumns(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("member", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".tenant_t_project_p_records", mtest.FirstBatch, bson.D{{Key: "title", Value: "safe"}, {Key: "internal", Value: "secret"}, {Key: "password", Value: "hash"}}))
		service := &DynamicService{repository: repositories.NewDynamicRepositoryWithDatabase(mt.DB)}
		result, err := service.ExportDynamicItems(context.Background(), ExportDynamicItemsInput{
			TenantID: "t", ProjectID: "p", AccessIdentityInput: AccessIdentityInput{UserRole: "member"},
			Container: &models.ContainerModel{SchemaName: "records", Fields: []models.Field{{Name: "title", Type: "string"}, {Name: "internal", Type: "string", IsAuthorized: true, AuthorizeRole: []string{"admin"}}, {Name: "password", Type: "string", IsHashed: true}}},
			Request:   requests.ExportRequest{SchemaName: "records"},
		})
		if err != nil {
			mt.Fatal(err)
		}
		workbook, err := excelize.OpenReader(bytes.NewReader(result.Content))
		if err != nil {
			mt.Fatal(err)
		}
		defer workbook.Close()
		rows, err := workbook.GetRows("Sheet1")
		if err != nil || len(rows) != 2 || len(rows[0]) != 1 || rows[1][0] != "safe" {
			mt.Fatalf("exported restricted columns: %#v %v", rows, err)
		}
	})
}

// Authorization regressions use emulated Redis and mock MongoDB only.
func TestCachedItemKeepsRoleProjectionsSeparate(t *testing.T) {
	redisServer := miniredis.RunT(t)
	old := configs.RedisClient
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	configs.RedisClient = client
	configs.RedisCircuitRecordSuccess()
	defer func() { configs.RedisClient = old; _ = client.Close() }()
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	for _, order := range [][]string{{"admin", "member"}, {"member", "admin"}, {"", "admin"}} {
		mt.Run(order[0]+" then "+order[1], func(mt *mtest.T) {
			id := primitive.NewObjectID()
			container := &models.ContainerModel{SchemaName: "records", Fields: []models.Field{{Name: "title", Type: "string"}, {Name: "internal", Type: "string", IsAuthorized: true, AuthorizeRole: []string{"admin"}}}, Redis: models.Redis{IsRedisCached: true, CacheTime: 10}}
			service := &DynamicService{repository: repositories.NewDynamicRepositoryWithDatabase(mt.DB), cache: cache.NewDynamicCache()}
			input := GetDynamicItemInput{TenantID: "t", ProjectID: "p", Schema: "records", ID: id.Hex(), Container: container}
			databaseRecord := func() {
				mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".tenant_t_project_p_records", mtest.FirstBatch, bson.D{{Key: "_id", Value: id}, {Key: "title", Value: "safe"}, {Key: "internal", Value: "restricted sample"}}))
			}
			for _, role := range order {
				databaseRecord()
				input.UserRole = role
				result, err := service.GetDynamicItem(context.Background(), input)
				if err != nil || (result.Item["internal"] != nil) != (role == "admin") {
					mt.Fatalf("role %q projection=%#v err=%v", role, result, err)
				}
			}
			// Both roles now hit their own independently populated cache entry.
			for _, role := range order {
				input.UserRole = role
				result, err := service.GetDynamicItem(context.Background(), input)
				if err != nil || !result.FromCache || (result.Item["internal"] != nil) != (role == "admin") {
					mt.Fatalf("cached role %q projection=%#v err=%v", role, result, err)
				}
			}
			// A field-policy edit cannot revive the previously cached projection.
			container.Fields[1].AuthorizeRole = []string{"member"}
			input.UserRole = order[0]
			databaseRecord()
			changed, err := service.GetDynamicItem(context.Background(), input)
			if err != nil || changed.FromCache || (changed.Item["internal"] != nil) != (input.UserRole == "member") {
				mt.Fatalf("policy edit used stale projection: %#v %v", changed, err)
			}
		})
	}
}

func TestCacheFillWaitStillProjectsRestrictedFields(t *testing.T) {
	redisServer := miniredis.RunT(t)
	old := configs.RedisClient
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	configs.RedisClient = client
	configs.RedisCircuitRecordSuccess()
	defer func() { configs.RedisClient = old; _ = client.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	id := primitive.NewObjectID()
	container := &models.ContainerModel{SchemaName: "records", Fields: []models.Field{{Name: "title"}, {Name: "internal", IsAuthorized: true, AuthorizeRole: []string{"admin"}}}, Redis: models.Redis{IsRedisCached: true, CacheTime: 10}}
	service := &DynamicService{cache: cache.NewDynamicCache()}
	key, ok := schemaCacheKey(ctx, "t", "p", "records", true, "GetDynamicModelItem", fieldProjectionCacheQuery(id.Hex(), container, "member"))
	if !ok {
		t.Fatal("cache unavailable")
	}
	lock, ok := utils.AcquireCacheFillLock(ctx, key)
	if !ok {
		t.Fatal("fill lock unavailable")
	}
	defer utils.ReleaseCacheFillLock(ctx, key, lock)
	seeded := make(chan struct{})
	go func() {
		time.Sleep(30 * time.Millisecond)
		service.cache.SetItem(ctx, key, map[string]interface{}{"_id": id, "title": "safe", "internal": "hidden"}, 10)
		close(seeded)
	}()
	result, err := service.GetDynamicItem(ctx, GetDynamicItemInput{TenantID: "t", ProjectID: "p", Schema: "records", ID: id.Hex(), Container: container, AccessIdentityInput: AccessIdentityInput{UserRole: "member"}})
	<-seeded
	if err != nil || !result.FromCache || result.Item["internal"] != nil || result.Item["title"] != "safe" {
		t.Fatalf("waited projection=%#v err=%v", result, err)
	}
}

func TestExportRejectsDifferentAuthorizedSchema(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("different metadata and data targets", func(mt *mtest.T) {
		target := "tenant_t_project_p_private_records"
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+"."+target, mtest.FirstBatch, bson.D{{Key: "title", Value: "private sample"}}))
		service := &DynamicService{repository: repositories.NewDynamicRepositoryWithDatabase(mt.DB)}
		_, err := service.ExportDynamicItems(context.Background(), ExportDynamicItemsInput{
			TenantID: "t", ProjectID: "p", Container: &models.ContainerModel{SchemaName: "public_records", Fields: []models.Field{{Name: "title", Type: "string"}}},
			Request: requests.ExportRequest{SchemaName: "private_records", Fields: []string{"title"}},
		})
		if err == nil {
			mt.Fatal("export accepted mismatched authorization target")
		}
		if len(mt.GetAllStartedEvents()) != 0 {
			mt.Fatal("database was queried before rejecting schema mismatch")
		}
	})
}

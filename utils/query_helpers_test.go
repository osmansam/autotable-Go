package utils

import (
	"context"
	"testing"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestQueryAndDecodeCollection(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	defer mt.Close()

	mt.Run("decodes and counts paginated query", func(mt *mtest.T) {
		mt.AddMockResponses(
			mtest.CreateCursorResponse(0, mt.Coll.Database().Name()+"."+mt.Coll.Name(), mtest.FirstBatch, bson.D{{Key: "name", Value: "Ada"}}),
			mtest.CreateSuccessResponse(bson.E{Key: "n", Value: int32(3)}),
		)
		pager := &Pager{Enabled: true, Page: 1, Limit: 2}
		got, err := QueryAndDecodeCollection(context.Background(), mt.Coll, "orders", bson.M{}, nil, pager)
		if err != nil || len(got) != 1 || got[0]["name"] != "Ada" || pager.TotalItems != 3 || pager.TotalPages != 2 {
			t.Fatalf("QueryAndDecodeCollection() = %#v, %v; pager = %#v", got, err, pager)
		}
	})

	mt.Run("estimates total when count fails", func(mt *mtest.T) {
		mt.AddMockResponses(
			mtest.CreateCursorResponse(0, mt.Coll.Database().Name()+"."+mt.Coll.Name(), mtest.FirstBatch, bson.D{{Key: "name", Value: "Ada"}}),
			mtest.CreateCommandErrorResponse(mtest.CommandError{Code: 1, Message: "count failed"}),
		)
		pager := &Pager{Enabled: true, Page: 2, Limit: 2}
		got, err := QueryAndDecodeCollection(context.Background(), mt.Coll, "orders", bson.M{}, nil, pager)
		if err != nil || len(got) != 1 || pager.TotalItems != 3 || pager.TotalPages != 2 {
			t.Fatalf("QueryAndDecodeCollection() = %#v, %v; pager = %#v", got, err, pager)
		}
	})
}

func TestExecuteDynamicPipeline(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	defer mt.Close()

	mt.Run("executes aggregation", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.Coll.Database().Name()+"."+mt.Coll.Name(), mtest.FirstBatch, bson.D{{Key: "state", Value: "open"}}))
		got, err := ExecuteDynamicPipeline(context.Background(), mt.Coll, models.PipelineStage{PipelineJSON: `[{"$match":{"state":"open"}}]`})
		if err != nil || len(got) != 1 || got[0]["state"] != "open" {
			t.Fatalf("ExecuteDynamicPipeline() = %#v, %v", got, err)
		}
	})
}

package repositories

import (
	"context"
	"time"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// NewDynamicRepositoryWithDatabase scopes data, metadata, counters, outbox, and
// transactions to the supplied database without initializing global app state.
// The caller owns the database client's lifecycle. database must not be nil.
func NewDynamicRepositoryWithDatabase(database *mongo.Database) *DynamicRepository {
	if database == nil {
		panic("dynamic repository requires a non-nil database")
	}
	collection := func(tenantID, projectID, name string) *mongo.Collection {
		if tenantID != "" && projectID != "" {
			name = utils.GetProjectCollectionName(tenantID, projectID, name)
		}
		return database.Collection(name)
	}
	return &DynamicRepository{
		database:         database,
		collection:       collection,
		globalCollection: func(name string) *mongo.Collection { return database.Collection(name) },
		getContainerModel: func(ctx context.Context, tenantID, projectID, schema string) (*models.ContainerModel, error) {
			if ctx == nil {
				ctx = context.Background()
			}
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			var container models.ContainerModel
			if err := collection(tenantID, projectID, "containers").FindOne(ctx, bson.M{"schemaName": schema}).Decode(&container); err != nil {
				return nil, err
			}
			container.SortFieldsByOrder()
			return &container, nil
		},
		getContainerModels: func(ctx context.Context) ([]models.ContainerModel, error) {
			if ctx == nil {
				ctx = context.Background()
			}
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			cursor, err := database.Collection("containers").Find(ctx, bson.M{})
			if err != nil {
				return nil, err
			}
			defer cursor.Close(ctx)
			var containers []models.ContainerModel
			if err := cursor.All(ctx, &containers); err != nil {
				return nil, err
			}
			return containers, nil
		},
		nextSequence: func(ctx context.Context, schema string) (int64, error) {
			var counter struct {
				Seq int64 `bson:"seq"`
			}
			err := database.Collection("counters").FindOneAndUpdate(ctx, bson.M{"_id": schema}, bson.M{"$inc": bson.M{"seq": 1}}, options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)).Decode(&counter)
			return counter.Seq, err
		},
	}
}

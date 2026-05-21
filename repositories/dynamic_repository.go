package repositories

import (
	"context"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type DynamicRepository struct{}

func NewDynamicRepository() *DynamicRepository {
	return &DynamicRepository{}
}

func (r *DynamicRepository) GetContainerModel(tenantID, projectID, schemaName string) (*models.ContainerModel, error) {
	return utils.GetContainerModel(tenantID, projectID, schemaName)
}

func (r *DynamicRepository) GetCollection(tenantID, projectID, schemaName string) *mongo.Collection {
	return utils.GetDynamicCollectionForProject(tenantID, projectID, schemaName)
}

func (r *DynamicRepository) CountByField(ctx context.Context, tenantID, projectID, schemaName, fieldName string, fieldValue interface{}) (int64, error) {
	return r.GetCollection(tenantID, projectID, schemaName).CountDocuments(ctx, bson.M{fieldName: fieldValue})
}

func (r *DynamicRepository) CountByFieldExcludingID(ctx context.Context, tenantID, projectID, schemaName, fieldName string, fieldValue interface{}, id interface{}) (int64, error) {
	filter := bson.M{
		fieldName: fieldValue,
		"_id":     bson.M{"$ne": id},
	}
	return r.GetCollection(tenantID, projectID, schemaName).CountDocuments(ctx, filter)
}

func (r *DynamicRepository) Insert(ctx context.Context, tenantID, projectID, schemaName string, item map[string]interface{}) (*mongo.InsertOneResult, error) {
	return r.GetCollection(tenantID, projectID, schemaName).InsertOne(ctx, item)
}

func (r *DynamicRepository) FindByID(ctx context.Context, tenantID, projectID, schemaName string, id interface{}) (bson.M, error) {
	var item bson.M
	err := r.GetCollection(tenantID, projectID, schemaName).FindOne(ctx, bson.M{"_id": id}).Decode(&item)
	return item, err
}

func (r *DynamicRepository) UpdateByID(ctx context.Context, tenantID, projectID, schemaName string, id interface{}, item map[string]interface{}) (*mongo.UpdateResult, error) {
	return r.GetCollection(tenantID, projectID, schemaName).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": item})
}

func (r *DynamicRepository) NextSequence(ctx context.Context, schemaName string) (int64, error) {
	return utils.GetNextSequence(ctx, schemaName)
}

package repositories

import (
	"context"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type DynamicRepository struct{}

func NewDynamicRepository() *DynamicRepository {
	return &DynamicRepository{}
}

func (r *DynamicRepository) GetContainerModel(tenantID, projectID, schemaName string) (*models.ContainerModel, error) {
	return utils.GetContainerModel(tenantID, projectID, schemaName)
}

func (r *DynamicRepository) GetAllContainerModels() ([]models.ContainerModel, error) {
	return utils.GetAllContainerModels()
}

func (r *DynamicRepository) GetCollection(tenantID, projectID, schemaName string) *mongo.Collection {
	return utils.GetDynamicCollectionForProject(tenantID, projectID, schemaName)
}

func (r *DynamicRepository) CountByField(ctx context.Context, tenantID, projectID, schemaName, fieldName string, fieldValue interface{}) (int64, error) {
	return r.GetCollection(tenantID, projectID, schemaName).CountDocuments(ctx, bson.M{fieldName: fieldValue})
}

func (r *DynamicRepository) CountByFieldIn(ctx context.Context, tenantID, projectID, schemaName, fieldName string, values []interface{}) (int64, error) {
	return r.GetCollection(tenantID, projectID, schemaName).CountDocuments(ctx, bson.M{fieldName: bson.M{"$in": values}})
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

func (r *DynamicRepository) InsertMany(ctx context.Context, tenantID, projectID, schemaName string, items []map[string]interface{}) (*mongo.InsertManyResult, error) {
	docs := make([]interface{}, len(items))
	for i, item := range items {
		docs[i] = item
	}
	return r.GetCollection(tenantID, projectID, schemaName).InsertMany(ctx, docs)
}

func (r *DynamicRepository) FindByID(ctx context.Context, tenantID, projectID, schemaName string, id interface{}) (bson.M, error) {
	var item bson.M
	err := r.GetCollection(tenantID, projectID, schemaName).FindOne(ctx, bson.M{"_id": id}).Decode(&item)
	return item, err
}

func (r *DynamicRepository) FindAll(ctx context.Context, tenantID, projectID, schemaName string, limit int64) ([]map[string]interface{}, error) {
	pager := utils.Pager{Enabled: false}
	opts := options.Find().SetLimit(limit)
	return utils.QueryAndDecode(ctx, tenantID, projectID, schemaName, bson.M{}, opts, &pager)
}

func (r *DynamicRepository) FindForSelection(ctx context.Context, tenantID, projectID, schemaName, fieldName string) ([]map[string]interface{}, error) {
	projection := bson.M{
		"_id":     1,
		fieldName: 1,
	}

	cursor, err := r.GetCollection(tenantID, projectID, schemaName).Find(ctx, bson.M{}, options.Find().SetProjection(projection))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var items []map[string]interface{}
	if err := cursor.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *DynamicRepository) DeleteByID(ctx context.Context, tenantID, projectID, schemaName string, id interface{}) (*mongo.DeleteResult, error) {
	return r.GetCollection(tenantID, projectID, schemaName).DeleteOne(ctx, bson.M{"_id": id})
}

func (r *DynamicRepository) DeleteManyByField(ctx context.Context, tenantID, projectID, schemaName, fieldName string, fieldValue interface{}) (*mongo.DeleteResult, error) {
	return r.GetCollection(tenantID, projectID, schemaName).DeleteMany(ctx, bson.M{fieldName: fieldValue})
}

func (r *DynamicRepository) UpdateByID(ctx context.Context, tenantID, projectID, schemaName string, id interface{}, item map[string]interface{}) (*mongo.UpdateResult, error) {
	return r.GetCollection(tenantID, projectID, schemaName).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": item})
}

func (r *DynamicRepository) NextSequence(ctx context.Context, schemaName string) (int64, error) {
	return utils.GetNextSequence(ctx, schemaName)
}

package repositories

import (
	"context"
	"errors"
	"fmt"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"time"
)

var ErrVisualComponentConflict = errors.New("visual design changed or is no longer available; reload before saving")

type VisualComponentRepository struct{ collection func() *mongo.Collection }

func NewVisualComponentRepository() *VisualComponentRepository {
	return &VisualComponentRepository{collection: func() *mongo.Collection { return configs.GetCollection("project_visual_components") }}
}
func (r *VisualComponentRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.collection().Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "projectId", Value: 1}, {Key: "updatedAt", Value: -1}}})
	return err
}
func visualComponentScope(tenant, project string) (bson.M, error) {
	if tenant == "" || project == "" {
		return nil, fmt.Errorf("tenant and project are required")
	}
	return bson.M{"tenantId": tenant, "projectId": project}, nil
}
func (r *VisualComponentRepository) List(ctx context.Context, tenant, project string) ([]models.VisualComponentDesign, error) {
	filter, err := visualComponentScope(tenant, project)
	if err != nil {
		return nil, err
	}
	cursor, err := r.collection().Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "updatedAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	values := []models.VisualComponentDesign{}
	err = cursor.All(ctx, &values)
	return values, err
}
func (r *VisualComponentRepository) Create(ctx context.Context, tenant, project string, value models.VisualComponentDesign) (models.VisualComponentDesign, error) {
	if _, err := visualComponentScope(tenant, project); err != nil {
		return value, err
	}
	value.ID = primitive.NewObjectID()
	value.TenantID = tenant
	value.ProjectID = project
	value.Revision = 1
	value.CreatedAt = time.Now().UTC()
	value.UpdatedAt = value.CreatedAt
	_, err := r.collection().InsertOne(ctx, value)
	return value, err
}
func (r *VisualComponentRepository) Update(ctx context.Context, tenant, project string, value models.VisualComponentDesign) (models.VisualComponentDesign, error) {
	filter, err := visualComponentScope(tenant, project)
	if err != nil {
		return value, err
	}
	if value.ID.IsZero() || value.Revision < 1 {
		return value, fmt.Errorf("design identity and revision are required")
	}
	filter["_id"] = value.ID
	filter["revision"] = value.Revision
	update := bson.M{"$set": bson.M{"name": value.Name, "value": value.Value, "updatedAt": time.Now().UTC()}, "$inc": bson.M{"revision": int64(1)}}
	var saved models.VisualComponentDesign
	err = r.collection().FindOneAndUpdate(ctx, filter, update, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&saved)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return saved, ErrVisualComponentConflict
	}
	return saved, err
}

// Clone copies definitions, never references, into a newly created project.
func (r *VisualComponentRepository) Clone(ctx context.Context, sourceTenant, sourceProject, targetTenant, targetProject string) error {
	values, err := r.List(ctx, sourceTenant, sourceProject)
	if err != nil {
		return err
	}
	for _, value := range values {
		if _, err = r.Create(ctx, targetTenant, targetProject, value); err != nil {
			return err
		}
	}
	return nil
}

package repositories

import (
	"context"
	"time"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ProjectLocalizationRepository struct {
	projects *mongo.Collection
}

func NewProjectLocalizationRepository() *ProjectLocalizationRepository {
	return NewProjectLocalizationRepositoryWithCollection(configs.GetCollection("projects"))
}

func NewProjectLocalizationRepositoryWithCollection(projects *mongo.Collection) *ProjectLocalizationRepository {
	return &ProjectLocalizationRepository{projects: projects}
}

func (r *ProjectLocalizationRepository) GetProject(ctx context.Context, tenantID, projectID primitive.ObjectID) (*models.Project, error) {
	var project models.Project
	if err := r.projects.FindOne(ctx, projectLocalizationFilter(tenantID, projectID)).Decode(&project); err != nil {
		return nil, err
	}
	project.ApplyLocaleDefaults()
	return &project, nil
}

func (r *ProjectLocalizationRepository) IncrementVersion(ctx context.Context, tenantID, projectID primitive.ObjectID) error {
	result, err := r.projects.UpdateOne(ctx, projectLocalizationFilter(tenantID, projectID), bson.M{
		"$inc": bson.M{"localizationVersion": 1},
		"$set": bson.M{"updatedAt": time.Now().UTC()},
	})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

func (r *ProjectLocalizationRepository) UpdateLocaleSettings(ctx context.Context, tenantID, projectID primitive.ObjectID, sourceLocale, defaultLocale string, enabledLocales []string) (*models.Project, error) {
	update := bson.M{
		"$set": bson.M{
			"sourceLocale": sourceLocale, "defaultLocale": defaultLocale,
			"enabledLocales": enabledLocales, "updatedAt": time.Now().UTC(),
		},
		"$inc": bson.M{"localizationVersion": 1},
	}
	var project models.Project
	err := r.projects.FindOneAndUpdate(
		ctx,
		projectLocalizationFilter(tenantID, projectID),
		update,
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&project)
	if err != nil {
		return nil, err
	}
	project.ApplyLocaleDefaults()
	return &project, nil
}

func projectLocalizationFilter(tenantID, projectID primitive.ObjectID) bson.M {
	return bson.M{"tenantId": tenantID, "_id": projectID}
}

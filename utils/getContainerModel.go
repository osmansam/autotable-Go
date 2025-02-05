package utils

import (
	"context"
	"time"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)
var containerCollection *mongo.Collection = configs.GetCollection( "containers")
// this function is used to get the container model from the database
func GetContainerModel(schemaName string) (*models.ContainerModel, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var containerModel models.ContainerModel
	err := containerCollection.FindOne(ctx, bson.M{"schemaName": schemaName}).Decode(&containerModel)
	if err != nil {
		return nil, err
	}
	return &containerModel, nil
}
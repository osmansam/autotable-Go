package utils

import (
	"os"

	"github.com/osmansam/autotableGo/configs"
	"go.mongodb.org/mongo-driver/mongo"
)

var (
	projectCollectionProvider   = GetProjectCollection
	containerCollectionProvider = GetContainerCollectionForProject
	dynamicCollectionProvider   = GetDynamicCollectionForProject
	globalCollectionProvider    = configs.GetCollection
	countersCollectionProvider  = func() *mongo.Collection {
		return configs.DB.Database(os.Getenv("COLLECTION_NAME")).Collection("counters")
	}
)

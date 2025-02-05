package configs

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var ctx = context.Background()

func ConnectRedis() *redis.Client {
  rdb := redis.NewClient(&redis.Options{
        Addr:     "localhost:6379", 
        Password: "",              
        DB:       0,              
    })

    _, err := rdb.Ping(ctx).Result()
    if err != nil {
        log.Fatal("Failed to connect to Redis", err)
    }

    fmt.Println("Connected to Redis")
    return rdb
}

func ConnectDB() *mongo.Client {
	client, err := mongo.NewClient(options.Client().ApplyURI(EnvMongoURI()))
	if err != nil {
		log.Fatal(err)
	}
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	err = client.Connect(ctx)
	if err != nil {
		log.Fatal(err)
	}

	//ping the database
	err = client.Ping(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Connected to MongoDB")
	return client
}


var (
	DB          *mongo.Client   = ConnectDB()
	RedisClient *redis.Client   = ConnectRedis()
	database    *mongo.Database = DB.Database("15-AUTOTABLE") 
)

//TODO: here database name can be put as an input and the function can be used to get the database
func GetCollection(collectionName string) *mongo.Collection {
	return database.Collection(collectionName)
}

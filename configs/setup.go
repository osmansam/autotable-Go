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
        Addr:     "localhost:6379", // or other Redis address
        Password: "",              // no password set
        DB:       0,               // use default DB
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

//Client instance
var DB *mongo.Client = ConnectDB()
var RedisClient *redis.Client = ConnectRedis()
//getting database collections
func GetCollection(client *mongo.Client, collectionName string) *mongo.Collection {
	collection := client.Database("15-AUTOTABLE").Collection(collectionName)
	return collection
}
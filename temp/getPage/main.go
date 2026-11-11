package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	uri := strings.TrimSuffix(os.Args[1], "/") + "/15-AUTOTABLE?retryWrites=true&w=majority" + "/15-AUTOTABLE?retryWrites=true&w=majority"+ "/15-AUTOTABLE?retryWrites=true&w=majority"
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		fmt.Println("connect:", err)
		return
	}
	defer client.Disconnect(ctx)
	names, err := client.Database("15-AUTOTABLE").ListCollectionNames(ctx, bson.D{})
	if err != nil {
		fmt.Println("list:", err)
		return
	}
	for _, name := range names {
		coll := client.Database("15-AUTOTABLE").Collection(name)
		count, _ := coll.EstimatedDocumentCount(ctx)
		if strings.Contains(strings.ToLower(name), "page") {
			fmt.Println("== collection", name, "count", count)
			cursor, _ := coll.Find(ctx, bson.M{})
			var docs []bson.M
			_ = cursor.All(ctx, &docs)
			for _, d := range docs {
				data, _ := json.Marshal(d)
				s := string(data)
				if strings.Contains(s, "requestBindings") || strings.Contains(s, "\"requests\"") || strings.Contains(s, "_outputs") {
					fmt.Println("NAME:", d["name"])
					fmt.Println(s[:min(len(s), 90000)])
					fmt.Println("----")
				}
			}
		}
	}
}

func min(a, b int) int { if a < b { return a }; return b }

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/osmansam/autotableGo/internal/collaborationmigration"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"os"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: autotable-collaboration preflight|apply --uri <explicit URI> --database <name> [--report-hash <reviewed hash>]")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	uri := flags.String("uri", "", "explicit database URI; never loaded from application .env")
	database := flags.String("database", "", "explicit database name")
	hash := flags.String("report-hash", "", "reviewed preflight hash")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if *uri == "" || *database == "" {
		return fmt.Errorf("explicit --uri and --database are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(*uri))
	if err != nil {
		return fmt.Errorf("database connection failed")
	}
	defer client.Disconnect(ctx)
	switch args[0] {
	case "preflight":
		report, err := collaborationmigration.Preflight(ctx, client.Database(*database))
		if err != nil {
			return fmt.Errorf("preflight failed")
		}
		return json.NewEncoder(os.Stdout).Encode(report)
	case "apply":
		return collaborationmigration.Apply(ctx, client.Database(*database), *hash)
	default:
		return fmt.Errorf("unknown command")
	}
}

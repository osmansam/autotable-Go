package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/osmansam/autotableGo/internal/upgrade"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"io"
	"os"
	"strings"
	"time"
)

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv)) }
func run(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) int {
	flags := flag.NewFlagSet("autotable-upgrade", flag.ContinueOnError)
	flags.SetOutput(stderr)
	action := flags.String("action", "audit", "audit (read-only), add-password-field, or set-password")
	database := flags.String("database", "", "explicit target MongoDB database")
	uriEnv := flags.String("uri-env", "AUTOTABLE_UPGRADE_MONGO_URI", "environment variable containing MongoDB connection URI")
	tenant := flags.String("tenant", "", "exact tenant ObjectID for a scoped metadata/enrollment write")
	project := flags.String("project", "", "exact project ObjectID for a scoped metadata/enrollment write")
	schema := flags.String("schema", "", "exact auth schema for a scoped write")
	user := flags.String("user", "", "verified application-user ObjectID for password enrollment/reset")
	passwordStdin := flags.Bool("password-stdin", false, "read an enrollment password from standard input; never from command arguments")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	fail := func(message string) int { fmt.Fprintln(stderr, message); return 1 }
	if flags.NArg() != 0 || *database == "" {
		return fail("An explicit --database is required; positional arguments are not accepted.")
	}
	if *action != "audit" && *action != "add-password-field" && *action != "set-password" {
		return fail("Unknown upgrade action.")
	}
	scope := upgrade.Scope{TenantID: *tenant, ProjectID: *project, Schema: *schema}
	if *action == "audit" && (*tenant != "" || *project != "" || *schema != "" || *user != "" || *passwordStdin) {
		return fail("Audit examines the selected database; enrollment/scope flags require an explicit write action.")
	}
	var password []byte
	if *action != "audit" {
		if err := scope.Validate(); err != nil {
			return fail("Writes require valid --tenant, --project, and --schema.")
		}
	}
	if *action == "set-password" {
		if !*passwordStdin {
			return fail("Enrollment requires --password-stdin and a verified --user.")
		}
		if _, err := primitive.ObjectIDFromHex(*user); err != nil {
			return fail("Enrollment requires a verified application-user ObjectID.")
		}
		var err error
		password, err = io.ReadAll(io.LimitReader(stdin, 74))
		if err != nil {
			return fail("Could not read enrollment password.")
		}
		password = []byte(strings.TrimSuffix(strings.TrimSuffix(string(password), "\n"), "\r"))
		defer func() {
			for i := range password {
				password[i] = 0
			}
		}()
		if err := upgrade.ValidateEnrollmentPassword(password); err != nil {
			return fail("Enrollment password must contain 8 to 72 bytes.")
		}
	}
	uri := getenv(*uriEnv)
	if uri == "" {
		return fail("The selected URI environment variable is not set; no implicit application configuration is loaded.")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri).SetServerSelectionTimeout(5*time.Second).SetConnectTimeout(5*time.Second))
	if err != nil {
		return fail("Could not configure the upgrade connection; verify the selected URI environment variable.")
	}
	defer func() {
		closeCtx, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		_ = client.Disconnect(closeCtx)
	}()
	if err := client.Ping(ctx, nil); err != nil {
		return fail("Could not reach the selected database; verify access and network configuration.")
	}
	db := client.Database(*database)
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	var result interface{}
	switch *action {
	case "audit":
		result, err = upgrade.Audit(ctx, db)
	case "add-password-field":
		var changed bool
		changed, err = upgrade.AddPasswordField(ctx, db, scope)
		result = map[string]interface{}{"changed": changed, "runEnrollmentAudit": true, "scope": map[string]string{"tenantId": scope.TenantID, "projectId": scope.ProjectID, "schema": scope.Schema}, "metadataCacheRefreshRequired": changed}
	case "set-password":
		err = upgrade.SetPassword(ctx, db, scope, *user, password)
		result = map[string]interface{}{"passwordUpdated": err == nil, "projectRefreshSessionsRevoked": err == nil, "existingAccessTokensExpireNormally": true}
	}
	if err != nil {
		return fail("Upgrade operation failed; check target scope, schema conflicts, permissions, and replica-set availability. No credential details are printed.")
	}
	if err := encoder.Encode(result); err != nil {
		return fail("Could not write upgrade report.")
	}
	return 0
}

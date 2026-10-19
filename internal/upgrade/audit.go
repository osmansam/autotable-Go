package upgrade

import (
	"context"
	"errors"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"regexp"
	"time"
)

type AuthFinding struct {
	TenantID                  string `json:"tenantId,omitempty"`
	ProjectID                 string `json:"projectId,omitempty"`
	Schema                    string `json:"schema"`
	PasswordLoginConfigured   bool   `json:"passwordLoginConfigured"`
	GoogleLoginEnabled        bool   `json:"googleLoginEnabled"`
	PublicRegistrationEnabled bool   `json:"publicRegistrationEnabled"`
	AccountsNeedingEnrollment int64  `json:"accountsNeedingEnrollment"`
	LegacyUnscoped            bool   `json:"legacyUnscoped"`
}
type Report struct {
	TransactionCapable             bool          `json:"transactionCapable"`
	Auth                           []AuthFinding `json:"auth"`
	LegacyRefreshSessions          int64         `json:"legacyRefreshSessions"`
	DuplicateSessionFamilies       int64         `json:"duplicateSessionFamilies"`
	DuplicateTranslationIdentities int64         `json:"duplicateTranslationIdentities"`
	SessionKeyIndexReady           bool          `json:"sessionKeyIndexReady"`
	SessionExpiryIndexReady        bool          `json:"sessionExpiryIndexReady"`
	PendingCronRuns                int64         `json:"pendingCronRuns"`
	FailedCronRuns                 int64         `json:"failedCronRuns"`
}

// Audit performs only reads; no indexes, passwords, or application data change.
func Audit(ctx context.Context, db *mongo.Database) (Report, error) {
	report := Report{Auth: []AuthFinding{}}
	var hello struct {
		SetName     string `bson:"setName"`
		Message     string `bson:"msg"`
		WireVersion int    `bson:"maxWireVersion"`
	}
	if err := db.Client().Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		return report, err
	}
	report.TransactionCapable = (hello.SetName != "" || hello.Message == "isdbgrid") && hello.WireVersion >= 8
	names, err := db.ListCollectionNames(ctx, bson.M{"name": bson.M{"$regex": `^(containers|tenant_[0-9a-f]{24}_project_[0-9a-f]{24}_containers)$`}})
	if err != nil {
		return report, err
	}
	pattern := regexp.MustCompile(`^tenant_([0-9a-f]{24})_project_([0-9a-f]{24})_containers$`)
	for _, name := range names {
		scope := Scope{}
		matches := pattern.FindStringSubmatch(name)
		if len(matches) == 3 {
			scope.TenantID = matches[1]
			scope.ProjectID = matches[2]
		}
		cursor, err := db.Collection(name).Find(ctx, bson.M{"isAuthContainer": true})
		if err != nil {
			return report, err
		}
		for cursor.Next(ctx) {
			var container models.ContainerModel
			if err := cursor.Decode(&container); err != nil {
				cursor.Close(ctx)
				return report, err
			}
			finding := AuthFinding{TenantID: scope.TenantID, ProjectID: scope.ProjectID, Schema: container.SchemaName, GoogleLoginEnabled: container.IsGoogleLoginActive, PublicRegistrationEnabled: container.IsRegisterActive, LegacyUnscoped: len(matches) != 3}
			identifier := false
			var invalid []bson.M
			for _, field := range container.Fields {
				if !field.IsLoginCredential {
					continue
				}
				if !field.IsHashed {
					identifier = true
					continue
				}
				invalid = append(invalid, bson.M{field.Name: bson.M{"$not": bson.M{"$regex": `^\$2[aby]\$[0-9]{2}\$[./A-Za-z0-9]{53}$`}}})
			}
			finding.PasswordLoginConfigured = identifier && len(invalid) > 0
			filter := bson.M{}
			if len(invalid) > 0 {
				filter["$or"] = invalid
			}
			collection := container.SchemaName
			if !finding.LegacyUnscoped {
				collection = scope.collection(container.SchemaName)
			}
			finding.AccountsNeedingEnrollment, err = db.Collection(collection).CountDocuments(ctx, filter, options.Count().SetMaxTime(5*time.Second))
			if err != nil {
				cursor.Close(ctx)
				return report, err
			}
			report.Auth = append(report.Auth, finding)
		}
		err = cursor.Err()
		cursor.Close(ctx)
		if err != nil {
			return report, err
		}
	}
	now := time.Now().UTC()
	activeLegacy := bson.M{"sessionKey": bson.M{"$exists": false}, "revokedAt": bson.M{"$exists": false}, "expiresAt": bson.M{"$gt": now}}
	report.LegacyRefreshSessions, err = db.Collection("auth_sessions").CountDocuments(ctx, activeLegacy, options.Count().SetMaxTime(5*time.Second))
	if err != nil {
		return report, err
	}
	report.DuplicateSessionFamilies, err = duplicateCount(ctx, db.Collection("auth_sessions"), activeLegacy, "$familyId")
	if err != nil {
		return report, err
	}
	report.DuplicateTranslationIdentities, err = duplicateCount(ctx, db.Collection("project_translations"), bson.M{}, bson.M{"tenantId": "$tenantId", "projectId": "$projectId", "locale": "$locale", "key": "$translationKey"})
	if err != nil {
		return report, err
	}
	indexes, err := db.Collection("auth_sessions").Indexes().List(ctx)
	if err == nil {
		for indexes.Next(ctx) {
			var index struct {
				Key    bson.M `bson:"key"`
				Unique bool   `bson:"unique"`
				Sparse bool   `bson:"sparse"`
				TTL    *int64 `bson:"expireAfterSeconds"`
			}
			if err := indexes.Decode(&index); err != nil {
				indexes.Close(ctx)
				return report, err
			}
			if len(index.Key) == 1 && index.Key["sessionKey"] != nil && index.Unique && index.Sparse {
				report.SessionKeyIndexReady = true
			}
			if len(index.Key) == 1 && index.Key["expiresAt"] != nil && index.TTL != nil && *index.TTL == 0 {
				report.SessionExpiryIndexReady = true
			}
		}
		err = indexes.Err()
		indexes.Close(ctx)
		if err != nil {
			return report, err
		}
	} else {
		var command mongo.CommandError
		if !errors.As(err, &command) || command.Code != 26 {
			return report, err
		}
	}
	report.PendingCronRuns, err = db.Collection("dynamic_outbox").CountDocuments(ctx, bson.M{"operation": models.DynamicOutboxOperationCronWorkflow, "status": bson.M{"$in": []string{models.DynamicOutboxStatusPending, models.DynamicOutboxStatusProcessing}}})
	if err != nil {
		return report, err
	}
	report.FailedCronRuns, err = db.Collection("dynamic_outbox").CountDocuments(ctx, bson.M{"operation": models.DynamicOutboxOperationCronWorkflow, "status": models.DynamicOutboxStatusFailed})
	return report, err
}
func duplicateCount(ctx context.Context, collection *mongo.Collection, filter bson.M, identity interface{}) (int64, error) {
	cursor, err := collection.Aggregate(ctx, mongo.Pipeline{bson.D{{Key: "$match", Value: filter}}, bson.D{{Key: "$group", Value: bson.M{"_id": identity, "n": bson.M{"$sum": 1}}}}, bson.D{{Key: "$match", Value: bson.M{"n": bson.M{"$gt": 1}}}}, bson.D{{Key: "$count", Value: "groups"}}}, options.Aggregate().SetMaxTime(5*time.Second).SetAllowDiskUse(false))
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)
	var row struct {
		Groups int64 `bson:"groups"`
	}
	if cursor.Next(ctx) {
		if err := cursor.Decode(&row); err != nil {
			return 0, err
		}
	}
	return row.Groups, cursor.Err()
}

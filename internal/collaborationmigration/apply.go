package collaborationmigration

import (
	"context"
	"errors"
	"fmt"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func Apply(ctx context.Context, db *mongo.Database, expectedReportHash string) error {
	report, err := Preflight(ctx, db)
	if err != nil {
		return err
	}
	if len(report.Conflicts) > 0 {
		return fmt.Errorf("preflight has %d unresolved conflicts", len(report.Conflicts))
	}
	if expectedReportHash == "" || report.Hash != expectedReportHash {
		return errors.New("preflight state changed; review a fresh report")
	}
	flags := configs.GetCollaborationConfig()
	if flags.MembersEnabled || flags.InvitationsEnabled || flags.RecoveryEnabled {
		return errors.New("pause all collaboration feature flags before migration")
	}
	var hello struct {
		SetName string `bson:"setName"`
		Msg     string `bson:"msg"`
	}
	if err := db.RunCommand(ctx, bson.M{"hello": 1}).Decode(&hello); err != nil {
		return err
	}
	if hello.SetName == "" && hello.Msg != "isdbgrid" {
		return errors.New("collaboration requires transaction-capable MongoDB")
	}
	marker := db.Collection("management_collaboration_migrations")
	if _, err := marker.UpdateOne(ctx, bson.M{"_id": "schema"}, bson.M{"$set": bson.M{"ready": false}}, options.Update().SetUpsert(true)); err != nil {
		return err
	}
	// The marker write drains transactions holding the readiness fence. Recheck
	// the reviewed state after that barrier before any normalization/index work.
	lockedReport, err := Preflight(ctx, db)
	if err != nil {
		return err
	}
	if lockedReport.Hash != expectedReportHash || len(lockedReport.Conflicts) > 0 {
		return errors.New("authorization state changed while pausing grants; review a fresh preflight")
	}
	s, err := readState(ctx, db)
	if err != nil {
		return err
	}
	if err := repositories.EnsureCollaborationIndexes(ctx, db); err != nil {
		return err
	}
	for _, user := range s.Users {
		result, err := db.Collection("users").UpdateOne(ctx, bson.M{"_id": user.ID, "email": user.Email}, bson.M{"$set": bson.M{"normalizedEmail": models.NormalizeManagementEmail(user.Email)}})
		if err != nil {
			return err
		}
		if result.MatchedCount != 1 {
			return errors.New("account changed during migration; review a fresh report")
		}
	}
	_, err = marker.UpdateOne(ctx, bson.M{"_id": "schema"}, bson.M{"$set": bson.M{"ready": true, "version": 1, "reportHash": report.Hash}}, options.Update().SetUpsert(true))
	return err
}

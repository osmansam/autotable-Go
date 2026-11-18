package services

import (
	"context"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
	"testing"
)

func TestCollaborationFeaturesRequireReviewedMigration(t *testing.T) {
	t.Setenv("COLLABORATION_MEMBERS_ENABLED", "true")
	t.Setenv("COLLABORATION_INVITATIONS_ENABLED", "false")
	t.Setenv("COLLABORATION_RECOVERY_ENABLED", "false")
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	for _, ready := range []bool{false, true} {
		name := "paused"
		if ready {
			name = "ready"
		}
		mt.Run(name, func(mt *mtest.T) {
			mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".management_collaboration_migrations", mtest.FirstBatch, bson.D{{Key: "ready", Value: ready}, {Key: "version", Value: 1}}))
			features, err := ReadCollaborationFeatures(context.Background(), mt.DB)
			if err != nil {
				mt.Fatal(err)
			}
			if features.MembersEnabled != ready || features.InvitationsEnabled || features.RecoveryEnabled {
				mt.Fatalf("unready feature enabled: %#v", features)
			}
		})
	}
}

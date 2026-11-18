package services

import (
	"context"
	"errors"
	"github.com/osmansam/autotableGo/configs"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type CollaborationFeatures struct {
	MembersEnabled     bool `json:"membersEnabled"`
	InvitationsEnabled bool `json:"invitationsEnabled"`
	RecoveryEnabled    bool `json:"recoveryEnabled"`
}

func ReadCollaborationFeatures(ctx context.Context, db *mongo.Database) (CollaborationFeatures, error) {
	flags := configs.GetCollaborationConfig()
	if !flags.MembersEnabled {
		return CollaborationFeatures{}, nil
	}
	var marker struct {
		Ready   bool `bson:"ready"`
		Version int  `bson:"version"`
	}
	err := db.Collection("management_collaboration_migrations").FindOne(ctx, bson.M{"_id": "schema"}).Decode(&marker)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return CollaborationFeatures{}, nil
	}
	if err != nil {
		return CollaborationFeatures{}, err
	}
	if !marker.Ready || marker.Version < 1 {
		return CollaborationFeatures{}, nil
	}
	return CollaborationFeatures{MembersEnabled: true, InvitationsEnabled: flags.InvitationsEnabled, RecoveryEnabled: flags.RecoveryEnabled}, nil
}

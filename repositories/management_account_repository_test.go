package repositories

import (
	"context"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
	"golang.org/x/crypto/bcrypt"
	"testing"
)

func TestCollaborationExistingAccountRequiresCredentials(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("password required without a tenant session", func(mt *mtest.T) {
		hash, _ := bcrypt.GenerateFromPassword([]byte("correct-existing-password"), bcrypt.MinCost)
		id := primitive.NewObjectID()
		doc := bson.D{{Key: "_id", Value: id}, {Key: "email", Value: "person@example.test"}, {Key: "normalizedEmail", Value: "person@example.test"}, {Key: "password", Value: string(hash)}, {Key: "isActive", Value: true}}
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".users", mtest.FirstBatch, doc))
		if _, err := NewManagementAccountRepository(mt.DB).AuthenticatePassword(context.Background(), "person@example.test", []byte("wrong-password")); err == nil {
			mt.Fatal("email alone authenticated existing account")
		}
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".users", mtest.FirstBatch, doc))
		account, err := NewManagementAccountRepository(mt.DB).AuthenticatePassword(context.Background(), "Person@EXAMPLE.test", []byte("correct-existing-password"))
		if err != nil || account.ID != id {
			mt.Fatalf("existing account proof failed: %v", err)
		}
		for _, event := range mt.GetAllStartedEvents() {
			if event.CommandName != "find" {
				mt.Fatalf("credential verification changed account: %s", event.CommandName)
			}
		}
	})
}
func TestCollaborationAccountEmailAmbiguityFailsClosed(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("legacy normalized collision", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".users", mtest.FirstBatch, bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "email", Value: "Person@example.test"}, {Key: "isActive", Value: true}}, bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "email", Value: "person@example.test"}, {Key: "isActive", Value: true}}))
		if _, err := NewManagementAccountRepository(mt.DB).AuthenticatePassword(context.Background(), models.NormalizeManagementEmail("Person@example.test"), []byte("some-password")); err == nil {
			mt.Fatal("ambiguous account was selected")
		}
	})
}

package upgrade

import (
	"context"
	"errors"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	"golang.org/x/crypto/bcrypt"
	"time"
)

func AddPasswordField(ctx context.Context, db *mongo.Database, scope Scope) (bool, error) {
	if err := scope.Validate(); err != nil {
		return false, err
	}
	collection := db.Collection(scope.collection("containers"))
	var raw bson.Raw
	if err := collection.FindOne(ctx, bson.M{"schemaName": scope.Schema, "isAuthContainer": true}).Decode(&raw); err != nil {
		return false, err
	}
	var container models.ContainerModel
	if err := bson.Unmarshal(raw, &container); err != nil {
		return false, err
	}
	field, needed, err := PlanPasswordField(container)
	if err != nil || !needed {
		return false, err
	}
	result, err := collection.UpdateOne(ctx, bson.M{"_id": raw.Lookup("_id"), "fields": raw.Lookup("fields"), "isAuthContainer": true}, bson.M{"$push": bson.M{"fields": field}, "$set": bson.M{"isRegisterActive": false}})
	if err != nil {
		return false, err
	}
	if result.MatchedCount != 1 {
		return false, errors.New("schema changed during migration; audit again")
	}
	return true, nil
}

// An operator must verify account ownership before using this explicit, scoped
// enrollment/reset operation. Password bytes are never logged or returned.
func SetPassword(ctx context.Context, db *mongo.Database, scope Scope, userID string, password []byte) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	id, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return errors.New("invalid user id")
	}
	if err := ValidateEnrollmentPassword(password); err != nil {
		return err
	}
	hashed, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	if err != nil {
		return errors.New("password hashing failed")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	session, err := db.Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)
	_, err = session.WithTransaction(ctx, func(tx mongo.SessionContext) (interface{}, error) {
		metadata := db.Collection(scope.collection("containers"))
		var container models.ContainerModel
		if err := metadata.FindOne(tx, bson.M{"schemaName": scope.Schema, "isAuthContainer": true}).Decode(&container); err != nil {
			return nil, err
		}
		supported := false
		for _, field := range container.Fields {
			if field.Name == "password" && field.Type == "string" && field.IsHashed && field.IsLoginCredential {
				supported = true
			}
		}
		if !supported {
			return nil, errors.New("required hashed password credential is not configured")
		}
		// Make enrollment conflict with simultaneous schema/auth changes.
		if _, err := metadata.UpdateOne(tx, bson.M{"_id": container.ID}, bson.M{"$inc": bson.M{"credentialEnrollmentFence": 1}}); err != nil {
			return nil, err
		}
		result, err := db.Collection(scope.collection(scope.Schema)).UpdateOne(tx, bson.M{"_id": id}, bson.M{"$set": bson.M{"password": string(hashed)}})
		if err != nil {
			return nil, err
		}
		if result.MatchedCount != 1 {
			return nil, errors.New("account does not exist in the selected scope")
		}
		_, err = db.Collection("auth_sessions").UpdateMany(tx, bson.M{"userId": id.Hex(), "tenantId": scope.TenantID, "projectId": scope.ProjectID, "scope": "project", "revokedAt": bson.M{"$exists": false}}, bson.M{"$set": bson.M{"revokedAt": time.Now().UTC(), "updatedAt": time.Now().UTC()}})
		return nil, err
	}, options.Transaction().SetReadConcern(readconcern.Snapshot()).SetWriteConcern(writeconcern.Majority()))
	return err
}

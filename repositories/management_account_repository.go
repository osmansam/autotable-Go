package repositories

import (
	"context"
	"errors"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
	"regexp"
)

var (
	ErrManagementAccountInvalid  = errors.New("invalid account credentials")
	ErrManagementAccountDisabled = errors.New("management account is disabled")
)

type ManagementAccountRepository struct{ db *mongo.Database }

func NewManagementAccountRepository(db *mongo.Database) *ManagementAccountRepository {
	return &ManagementAccountRepository{db: db}
}

func ManagementEmailLookup(email string) bson.M {
	normalized := models.NormalizeManagementEmail(email)
	return bson.M{"$or": bson.A{
		bson.M{"normalizedEmail": normalized},
		bson.M{"email": primitive.Regex{Pattern: "^" + regexp.QuoteMeta(normalized) + "$", Options: "i"}},
	}}
}

type managementAccountPassword struct {
	models.User  `bson:",inline"`
	PasswordHash string `bson:"password"`
}

func (r *ManagementAccountRepository) AuthenticatePassword(ctx context.Context, email string, password []byte) (*models.User, error) {
	if email == "" || len(password) == 0 || len(password) > 72 {
		return nil, ErrManagementAccountInvalid
	}
	cursor, err := r.db.Collection("users").Find(ctx, ManagementEmailLookup(email), options.Find().SetLimit(2))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var accounts []managementAccountPassword
	if err := cursor.All(ctx, &accounts); err != nil {
		return nil, err
	}
	if len(accounts) != 1 || accounts[0].PasswordHash == "" {
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"), password)
		return nil, ErrManagementAccountInvalid
	}
	if err := bcrypt.CompareHashAndPassword([]byte(accounts[0].PasswordHash), password); err != nil {
		return nil, ErrManagementAccountInvalid
	}
	if !accounts[0].IsActive {
		return nil, ErrManagementAccountDisabled
	}
	user := accounts[0].User
	user.NormalizedEmail = models.NormalizeManagementEmail(user.Email)
	return &user, nil
}
func (r *ManagementAccountRepository) GetByID(ctx context.Context, id primitive.ObjectID) (*models.User, error) {
	var account models.User
	err := r.db.Collection("users").FindOne(ctx, bson.M{"_id": id}, options.FindOne().SetProjection(bson.M{"password": 0})).Decode(&account)
	if account.NormalizedEmail == "" {
		account.NormalizedEmail = models.NormalizeManagementEmail(account.Email)
	}
	return &account, err
}

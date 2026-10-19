package controllers

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func TestProjectPasswordLoginCreatesOneSession(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-project-secret")
	hash, err := utils.HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("successful login", func(mt *mtest.T) {
		id := primitive.NewObjectID()
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+"."+mt.Coll.Name(), mtest.FirstBatch, bson.D{{Key: "_id", Value: id}, {Key: "email", Value: "person@example.com"}, {Key: "password", Value: hash}, {Key: "role", Value: "user"}}))
		sessions := 0
		dependencies := projectLoginDependencies{
			loadContainer: func(context.Context, string, string) (models.ContainerModel, error) { return secureLoginFixture(), nil },
			collection:    func(string, string, string) *mongo.Collection { return mt.Coll },
			startSession:  func(context.Context, *utils.TokenDetails) error { sessions++; return nil },
			logLogin:      func(context.Context, string, string, *models.AuditUser, string, string) error { return nil },
		}
		app := fiber.New()
		app.Post("/", func(c *fiber.Ctx) error {
			c.Locals("tenantID", "tenant")
			c.Locals("projectID", "project")
			return loginProjectUser(c, dependencies)
		})
		request := httptest.NewRequest("POST", "/", strings.NewReader(`{"email":"person@example.com","password":"correct-password"}`))
		request.Header.Set("Content-Type", "application/json")
		response, err := app.Test(request)
		if err != nil {
			mt.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode != 200 || sessions != 1 {
			mt.Fatalf("status=%d sessions=%d body=%s", response.StatusCode, sessions, body)
		}
		if strings.Contains(string(body), hash) || strings.Contains(string(body), "correct-password") {
			mt.Fatal("credential leaked in response")
		}
		if len(response.Cookies()) == 0 {
			mt.Fatal("no login cookies")
		}
	})
}

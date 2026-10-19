package utils

import (
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"testing"
)

func TestNestedTypedBSONArraysAreProjected(t *testing.T) {
	fields := []models.Field{{Name: "rows", Children: []models.Field{{Name: "name"}, {Name: "private", IsAuthorized: true, AuthorizeRole: []string{"admin"}}}}}
	doc := bson.M{"rows": []bson.M{{"name": "safe", "private": "hidden"}}}
	got := FilterDocumentFields(doc, fields, "member")
	if row := got["rows"].([]bson.M)[0]; row["private"] != nil || row["name"] != "safe" {
		t.Fatalf("typed BSON array was not projected: %#v", got)
	}
}

func TestFieldProjectionFiltersJSONMapsAndArrayChildren(t *testing.T) {
	fields := []models.Field{{Name: "profile", Children: []models.Field{
		{Name: "name"}, {Name: "private", IsAuthorized: true, AuthorizeRole: []string{"admin"}},
	}}, {Name: "rows", Children: []models.Field{
		{Name: "name"}, {Name: "private", IsAuthorized: true, AuthorizeRole: []string{"admin"}},
	}}}
	doc := bson.M{"profile": map[string]interface{}{"name": "safe", "private": "hidden"}, "rows": []interface{}{map[string]interface{}{"name": "safe", "private": "hidden"}}}
	got := FilterDocumentFields(doc, fields, "member")
	profile := got["profile"].(map[string]interface{})
	row := got["rows"].([]interface{})[0].(map[string]interface{})
	if profile["private"] != nil || row["private"] != nil {
		t.Fatalf("nested restricted data: %#v", got)
	}
	if profile["name"] != "safe" || row["name"] != "safe" {
		t.Fatalf("lost allowed data: %#v", got)
	}
	if doc["profile"].(map[string]interface{})["private"] != "hidden" {
		t.Fatal("projection mutated input")
	}
}

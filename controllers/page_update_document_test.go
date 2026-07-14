package controllers

import (
	"reflect"
	"testing"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
)

func TestPageUpdateDocumentIncludesEmptyFiltersWhenRequestContainsFilters(t *testing.T) {
	doc, err := pageUpdateDocument(models.PageModel{
		Name:    "Reports",
		Slug:    "reports",
		Filters: []models.PageFilterDefinition{},
	}, []byte(`{"name":"Reports","slug":"reports","filters":[]}`))
	if err != nil {
		t.Fatalf("pageUpdateDocument() error = %v", err)
	}

	updateSet, ok := doc["$set"].(bson.M)
	if !ok {
		t.Fatalf("$set = %T, want bson.M", doc["$set"])
	}
	filters, ok := updateSet["filters"]
	if !ok {
		t.Fatal("filters was omitted from update document")
	}
	if reflect.ValueOf(filters).Len() != 0 {
		t.Fatalf("filters length = %d, want 0", reflect.ValueOf(filters).Len())
	}
}

func TestPageUpdateDocumentOmitsNilFiltersWhenRequestOmitsFilters(t *testing.T) {
	doc, err := pageUpdateDocument(models.PageModel{
		Name: "Reports",
		Slug: "reports",
	}, []byte(`{"name":"Reports","slug":"reports"}`))
	if err != nil {
		t.Fatalf("pageUpdateDocument() error = %v", err)
	}

	updateSet, ok := doc["$set"].(bson.M)
	if !ok {
		t.Fatalf("$set = %T, want bson.M", doc["$set"])
	}
	if _, ok := updateSet["filters"]; ok {
		t.Fatal("filters should be omitted when the request omits filters")
	}
}

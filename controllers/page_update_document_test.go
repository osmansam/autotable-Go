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

func TestPageUpdateDocumentIncludesTableNestedRows(t *testing.T) {
	doc, err := pageUpdateDocument(models.PageModel{
		Name: "Orders",
		Sections: []models.Section{{
			Type: models.SectionTypeComponent,
			Component: &models.ComponentBlock{
				ID:   "orders-table",
				Type: models.ComponentTypeTable,
				Table: &models.TableComponentConfig{
					NestedRows: &models.TableNestedRowsConfig{
						Enabled: true,
						Field:   "product",
						Header:  "Products",
						Columns: []models.TableNestedRowColumnConfig{
							{Field: "productDavinciId", DisplayName: "Davinci ID", Type: "number"},
							{Field: "productId", DisplayName: "Product ID"},
							{Field: "quantity", DisplayName: "Quantity", Type: "number"},
						},
					},
				},
			},
		}},
	}, []byte(`{"name":"Orders","sections":[]}`))
	if err != nil {
		t.Fatalf("pageUpdateDocument() error = %v", err)
	}

	updateSet, ok := doc["$set"].(bson.M)
	if !ok {
		t.Fatalf("$set = %T, want bson.M", doc["$set"])
	}
	sections, ok := updateSet["sections"].(bson.A)
	if !ok || len(sections) != 1 {
		t.Fatalf("sections = %#v, want one section", updateSet["sections"])
	}
	section, ok := sections[0].(bson.M)
	if !ok {
		t.Fatalf("section = %T, want bson.M", sections[0])
	}
	component, ok := section["component"].(bson.M)
	if !ok {
		t.Fatalf("component = %#v, want bson.M", section["component"])
	}
	table, ok := component["table"].(bson.M)
	if !ok {
		t.Fatalf("table = %#v, want bson.M", component["table"])
	}
	nestedRows, ok := table["nestedRows"].(bson.M)
	if !ok {
		t.Fatalf("nestedRows = %#v, want bson.M", table["nestedRows"])
	}
	if nestedRows["field"] != "product" {
		t.Fatalf("nestedRows.field = %#v, want product", nestedRows["field"])
	}
}

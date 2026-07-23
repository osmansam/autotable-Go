package validators

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestPrepareCreateItem(t *testing.T) {
	id := primitive.NewObjectID()
	container := &models.ContainerModel{Fields: []models.Field{
		{Name: "name", Type: "string"},
		{Name: "ownerId", Type: "objectId"},
		{Name: "createdAt", Type: "string"},
		{Name: "updatedAt", Type: "string"},
	}}
	item := map[string]interface{}{"name": "order", "ownerId": id.Hex(), "unknown": "removed"}

	if err := PrepareCreateItem("tenant", "project", container, item); err != nil {
		t.Fatalf("PrepareCreateItem() error = %v", err)
	}
	if item["ownerId"] != id {
		t.Fatalf("ownerId = %#v, want %s", item["ownerId"], id.Hex())
	}
	if _, ok := item["unknown"]; ok {
		t.Fatal("unknown field was not removed")
	}
	if item["createdAt"] == "" || item["updatedAt"] == "" {
		t.Fatalf("timestamps = (%#v, %#v)", item["createdAt"], item["updatedAt"])
	}
}

func TestPrepareCreateItemPreservesDateTime(t *testing.T) {
	container := &models.ContainerModel{Fields: []models.Field{
		{Name: "deliveredAt", Type: "date"},
	}}
	item := map[string]interface{}{
		"deliveredAt": "2026-07-17T15:42:13Z",
	}

	if err := PrepareCreateItem("", "", container, item); err != nil {
		t.Fatalf("PrepareCreateItem() error = %v", err)
	}
	got, ok := item["deliveredAt"].(time.Time)
	if !ok {
		t.Fatalf("deliveredAt = %#v, want time.Time", item["deliveredAt"])
	}
	want := time.Date(2026, 7, 17, 15, 42, 13, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("deliveredAt = %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestPrepareUpdateFieldsPreservesDateTime(t *testing.T) {
	container := &models.ContainerModel{Fields: []models.Field{
		{Name: "deliveredAt", Type: "date"},
	}}
	item := map[string]interface{}{
		"deliveredAt": "2026-07-17T15:42:13Z",
	}

	if err := PrepareUpdateFields(container, item); err != nil {
		t.Fatalf("PrepareUpdateFields() error = %v", err)
	}
	got, ok := item["deliveredAt"].(time.Time)
	if !ok {
		t.Fatalf("deliveredAt = %#v, want time.Time", item["deliveredAt"])
	}
	want := time.Date(2026, 7, 17, 15, 42, 13, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("deliveredAt = %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestPrepareCreateItemsUsesSharedTimestampAndReportsIndex(t *testing.T) {
	container := &models.ContainerModel{Fields: []models.Field{
		{Name: "email", Type: "string", IsLoginCredential: true},
		{Name: "createdAt", Type: "string"},
		{Name: "updatedAt", Type: "string"},
	}}
	items := []map[string]interface{}{{"email": "one@example.com"}, {"email": "two@example.com"}}
	if err := PrepareCreateItems("", "", container, items); err != nil {
		t.Fatalf("PrepareCreateItems() error = %v", err)
	}
	if items[0]["createdAt"] != items[1]["createdAt"] || items[0]["updatedAt"] != items[1]["updatedAt"] {
		t.Fatalf("timestamps differ: %#v", items)
	}

	err := PrepareCreateItems("", "", container, []map[string]interface{}{{"email": "ok"}, {}})
	if err == nil || !strings.Contains(err.Error(), "validation failed for item at index 1") {
		t.Fatalf("PrepareCreateItems() error = %v", err)
	}
}

func TestPrepareUpdateFields(t *testing.T) {
	container := &models.ContainerModel{Fields: []models.Field{
		{Name: "name", Type: "string"},
		{Name: "updatedAt", Type: "string"},
	}}
	item := map[string]interface{}{"name": "updated", "unknown": "removed"}
	if err := PrepareUpdateFields(container, item); err != nil {
		t.Fatalf("PrepareUpdateFields() error = %v", err)
	}
	if _, ok := item["unknown"]; ok || item["updatedAt"] == "" {
		t.Fatalf("PrepareUpdateFields() = %#v", item)
	}
}

func TestPrepareMergedUpdateItemConvertsObjectIDs(t *testing.T) {
	id := primitive.NewObjectID()
	secondID := primitive.NewObjectID()
	container := &models.ContainerModel{Fields: []models.Field{
		{Name: "ownerId", Type: "objectId"},
		{Name: "memberIds", Type: "objectIdArray"},
	}}
	existing := map[string]interface{}{}
	update := map[string]interface{}{"ownerId": id.Hex(), "memberIds": []interface{}{secondID.Hex(), "invalid", id}}
	if err := PrepareMergedUpdateItem("", "", container, existing, update); err != nil {
		t.Fatalf("PrepareMergedUpdateItem() error = %v", err)
	}
	want := map[string]interface{}{"ownerId": id, "memberIds": []primitive.ObjectID{secondID, id}}
	if !reflect.DeepEqual(existing, want) {
		t.Fatalf("PrepareMergedUpdateItem() = %#v, want %#v", existing, want)
	}
}

func TestEquationErrors(t *testing.T) {
	cause := errors.New("cause")
	fieldErr := &EquationFieldError{FieldName: "total", Err: cause}
	if !errors.Is(fieldErr, cause) || !strings.Contains(fieldErr.Error(), "field total") {
		t.Fatalf("EquationFieldError = %v", fieldErr)
	}
	itemErr := &EquationItemFieldError{FieldName: "total", Index: 2, Err: cause}
	if !errors.Is(itemErr, cause) || !strings.Contains(itemErr.Error(), "item 2") {
		t.Fatalf("EquationItemFieldError = %v", itemErr)
	}
}

func TestPrepareEquationFields(t *testing.T) {
	container := &models.ContainerModel{Fields: []models.Field{
		{Name: "price", Type: "float"},
		{Name: "quantity", Type: "int"},
		{Name: "total", Type: "float", Equation: "price * quantity"},
	}}
	item := map[string]interface{}{"price": 2.5, "quantity": 4}
	if err := PrepareCreateItem("", "", container, item); err != nil || item["total"] != float64(10) {
		t.Fatalf("PrepareCreateItem() item = %#v, error = %v", item, err)
	}
	if err := PrepareMergedUpdateItem("", "", container, item, map[string]interface{}{"quantity": 5}); err != nil || item["total"] != float64(12.5) {
		t.Fatalf("PrepareMergedUpdateItem() item = %#v, error = %v", item, err)
	}

	invalid := &models.ContainerModel{Fields: []models.Field{{Name: "total", Type: "float", Equation: "missing + 1"}}}
	if err := PrepareCreateItem("", "", invalid, map[string]interface{}{}); err == nil {
		t.Fatal("PrepareCreateItem(invalid equation) error = nil")
	}
	if err := PrepareCreateItems("", "", invalid, []map[string]interface{}{{}}); err == nil {
		t.Fatal("PrepareCreateItems(invalid equation) error = nil")
	}
}

func TestPrepareMergedUpdateItemConvertsStringObjectIDArray(t *testing.T) {
	id := primitive.NewObjectID()
	item := map[string]interface{}{}
	container := &models.ContainerModel{Fields: []models.Field{{Name: "memberIds", Type: "objectIdArray"}}}
	if err := PrepareMergedUpdateItem("", "", container, item, map[string]interface{}{"memberIds": []string{id.Hex(), "invalid"}}); err != nil {
		t.Fatalf("PrepareMergedUpdateItem() error = %v", err)
	}
	if !reflect.DeepEqual(item["memberIds"], []primitive.ObjectID{id}) {
		t.Fatalf("memberIds = %#v", item["memberIds"])
	}
}

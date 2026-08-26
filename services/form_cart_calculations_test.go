package services

import (
	"reflect"
	"strings"
	"testing"

	"github.com/osmansam/autotableGo/models"
)

func calculationPrecision(value int) *int      { return &value }
func calculationNumber(value float64) *float64 { return &value }

func calculatedOrderForm() models.FormComponentConfig {
	return models.FormComponentConfig{
		ObjectLists: []models.FormObjectListConfig{{
			Key: "items",
			ItemCalculations: []models.FormItemCalculationConfig{{
				Operation: "multiply", Inputs: []string{"unitPrice", "quantity"}, TargetField: "lineTotal", Precision: calculationPrecision(2),
			}},
		}},
		Summaries: []models.FormSummaryConfig{
			{Key: "subtotal", Operation: "sum", ObjectListKey: "items", SourceField: "lineTotal", TargetField: "subtotal", Format: &models.FormValueFormatConfig{Precision: calculationPrecision(2)}},
			{Key: "total", Operation: "copy", SourceField: "subtotal", TargetField: "total", Format: &models.FormValueFormatConfig{Precision: calculationPrecision(2)}},
		},
	}
}

func TestEvaluateFormCartSharedFixture(t *testing.T) {
	record := map[string]interface{}{"items": []interface{}{
		map[string]interface{}{"unitPrice": 19.99, "quantity": 3},
		map[string]interface{}{"unitPrice": 5.25, "quantity": 2},
	}}
	got, err := EvaluateFormCart(calculatedOrderForm(), record)
	if err != nil {
		t.Fatal(err)
	}
	items := got["items"].([]interface{})
	if items[0].(map[string]interface{})["lineTotal"] != 59.97 || items[1].(map[string]interface{})["lineTotal"] != 10.5 {
		t.Fatalf("items = %#v", items)
	}
	if got["subtotal"] != 70.47 || got["total"] != 70.47 {
		t.Fatalf("summaries = %#v", got)
	}
	if _, exists := record["subtotal"]; exists {
		t.Fatal("input record was mutated")
	}
}

func TestEvaluateFormCartPrecisionAndEmptyList(t *testing.T) {
	form := calculatedOrderForm()
	form.ObjectLists[0].ItemCalculations[0].Precision = calculationPrecision(6)
	got, err := EvaluateFormCart(form, map[string]interface{}{"items": []interface{}{map[string]interface{}{"unitPrice": 10, "quantity": 2.0 / 3.0}}})
	if err != nil || got["items"].([]interface{})[0].(map[string]interface{})["lineTotal"] != 6.666667 {
		t.Fatalf("EvaluateFormCart() = %#v, %v", got, err)
	}
	empty, err := EvaluateFormCart(calculatedOrderForm(), map[string]interface{}{"items": []interface{}{}})
	if err != nil || !reflect.DeepEqual(empty["subtotal"], float64(0)) {
		t.Fatalf("empty = %#v, %v", empty, err)
	}
}

func TestEvaluateFormCartRejectsInvalidValues(t *testing.T) {
	for _, quantity := range []interface{}{0, -1, "bad"} {
		_, err := EvaluateFormCart(calculatedOrderForm(), map[string]interface{}{"items": []interface{}{map[string]interface{}{"unitPrice": 10, "quantity": quantity}}})
		if err == nil || !strings.Contains(err.Error(), "FORM_INVALID_QUANTITY") {
			t.Fatalf("quantity %#v error = %v", quantity, err)
		}
	}
	_, err := EvaluateFormCart(calculatedOrderForm(), map[string]interface{}{"items": []interface{}{map[string]interface{}{"quantity": 1}}})
	if err == nil || !strings.Contains(err.Error(), "FORM_PRODUCT_PRICE_MISSING") {
		t.Fatalf("missing price error = %v", err)
	}
}

func TestEvaluateFormCartQuantityDiscount(t *testing.T) {
	form := models.FormComponentConfig{
		ObjectLists: []models.FormObjectListConfig{{
			Key: "items",
			ItemCalculations: []models.FormItemCalculationConfig{{
				Operation: "quantityDiscount", Inputs: []string{"unitPrice", "quantity"},
				OriginalTargetField: "originalLineTotal", TargetField: "lineTotal",
				MinimumQuantity: calculationNumber(6), DiscountPercentage: calculationNumber(30), Precision: calculationPrecision(2),
			}},
		}},
		Summaries: []models.FormSummaryConfig{{
			Key: "total", Operation: "sum", ObjectListKey: "items", SourceField: "lineTotal", TargetField: "total",
			Format: &models.FormValueFormatConfig{Precision: calculationPrecision(2)},
		}},
	}
	record := map[string]interface{}{"items": []interface{}{
		map[string]interface{}{"unitPrice": 100, "quantity": 5, "originalLineTotal": 1, "lineTotal": 1},
		map[string]interface{}{"unitPrice": 100, "quantity": 6, "originalLineTotal": 1, "lineTotal": 1},
		map[string]interface{}{"unitPrice": 100, "quantity": 7, "originalLineTotal": 1, "lineTotal": 1},
	}}

	got, err := EvaluateFormCart(form, record)
	if err != nil {
		t.Fatal(err)
	}
	items := got["items"].([]interface{})
	want := [][2]float64{{500, 500}, {600, 420}, {700, 490}}
	for index, expected := range want {
		item := items[index].(map[string]interface{})
		if item["originalLineTotal"] != expected[0] || item["lineTotal"] != expected[1] {
			t.Fatalf("item %d = %#v, want original=%v line=%v", index, item, expected[0], expected[1])
		}
	}
	if got["total"] != float64(1410) {
		t.Fatalf("total = %#v, want 1410", got["total"])
	}
	if record["items"].([]interface{})[1].(map[string]interface{})["lineTotal"] != 1 {
		t.Fatal("input record was mutated")
	}
}

func TestEvaluateFormCartUsesHighestReachedDiscountTierPerRow(t *testing.T) {
	form := models.FormComponentConfig{
		ObjectLists: []models.FormObjectListConfig{{
			Key: "items",
			ItemCalculations: []models.FormItemCalculationConfig{{
				Operation: "quantityDiscount", Inputs: []string{"unitPrice", "quantity"},
				OriginalTargetField: "originalLineTotal", TargetField: "lineTotal",
				DiscountTiers: []models.FormQuantityDiscountTierConfig{
					{MinimumQuantity: calculationNumber(6), DiscountPercentage: calculationNumber(30)},
					{MinimumQuantity: calculationNumber(10), DiscountPercentage: calculationNumber(40)},
				},
				Precision: calculationPrecision(2),
			}},
		}},
		Summaries: []models.FormSummaryConfig{{
			Key: "total", Operation: "sum", ObjectListKey: "items", SourceField: "lineTotal", TargetField: "total",
			Format: &models.FormValueFormatConfig{Precision: calculationPrecision(2)},
		}},
	}
	quantities := []int{3, 6, 8, 10, 12}
	items := make([]interface{}, 0, len(quantities))
	for _, quantity := range quantities {
		items = append(items, map[string]interface{}{"unitPrice": 100, "quantity": quantity})
	}

	got, err := EvaluateFormCart(form, map[string]interface{}{"items": items})
	if err != nil {
		t.Fatal(err)
	}
	want := [][2]float64{{300, 300}, {600, 420}, {800, 560}, {1000, 600}, {1200, 720}}
	for index, expected := range want {
		item := got["items"].([]interface{})[index].(map[string]interface{})
		if item["originalLineTotal"] != expected[0] || item["lineTotal"] != expected[1] {
			t.Fatalf("item %d = %#v, want original=%v line=%v", index, item, expected[0], expected[1])
		}
	}
	if got["total"] != float64(2600) {
		t.Fatalf("total = %#v, want 2600", got["total"])
	}
}

func TestEvaluateFormCartQuantityDiscountRoundingAndFullDiscount(t *testing.T) {
	form := models.FormComponentConfig{ObjectLists: []models.FormObjectListConfig{{
		Key: "items",
		ItemCalculations: []models.FormItemCalculationConfig{{
			Operation: "quantityDiscount", Inputs: []string{"unitPrice", "quantity"},
			OriginalTargetField: "originalLineTotal", TargetField: "lineTotal",
			MinimumQuantity: calculationNumber(6), DiscountPercentage: calculationNumber(30), Precision: calculationPrecision(2),
		}},
	}}}

	got, err := EvaluateFormCart(form, map[string]interface{}{"items": []interface{}{
		map[string]interface{}{"unitPrice": 19.99, "quantity": 6},
	}})
	if err != nil {
		t.Fatal(err)
	}
	item := got["items"].([]interface{})[0].(map[string]interface{})
	if item["originalLineTotal"] != 119.94 || item["lineTotal"] != 83.96 {
		t.Fatalf("rounded item = %#v", item)
	}

	form.ObjectLists[0].ItemCalculations[0].DiscountPercentage = calculationNumber(100)
	got, err = EvaluateFormCart(form, map[string]interface{}{"items": []interface{}{
		map[string]interface{}{"unitPrice": 19.99, "quantity": 6},
	}})
	if err != nil || got["items"].([]interface{})[0].(map[string]interface{})["lineTotal"] != float64(0) {
		t.Fatalf("full discount = %#v, %v", got, err)
	}
}

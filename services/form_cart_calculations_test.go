package services

import (
	"reflect"
	"strings"
	"testing"

	"github.com/osmansam/autotableGo/models"
)

func calculationPrecision(value int) *int { return &value }

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

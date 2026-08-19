package services

import (
	"net/http"
	"testing"

	"github.com/osmansam/autotableGo/models"
)

func authoritativeOrderForm() models.FormComponentConfig {
	form := calculatedOrderForm()
	form.SchemaName = "davinciOrder"
	form.Fields = []models.FormFieldConfig{{ActionFormFieldConfig: models.ActionFormFieldConfig{FormKey: "productId", Type: "select", OptionsSource: "schema", SourceSchemaName: "product", SourceValueField: "_id"}}}
	form.ObjectLists[0].FieldMappings = []models.FormFieldMappingConfig{{SourceFormKey: "productId", SourceField: "price", TargetField: "unitPrice", Required: true}}
	return form
}

func TestVerifyAuthoritativeFormCartReplacesFinancialValues(t *testing.T) {
	record := map[string]interface{}{"items": []interface{}{map[string]interface{}{
		"productId": "p1", "quantity": 3, "unitPrice": 19.99, "lineTotal": 59.97,
	}}, "subtotal": 59.97, "total": 59.97}
	got, err := verifyAuthoritativeFormCart(authoritativeOrderForm(), record, map[string]map[string]interface{}{
		"p1": {"_id": "p1", "price": 19.99},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["total"] != 59.97 || got["items"].([]interface{})[0].(map[string]interface{})["unitPrice"] != 19.99 {
		t.Fatalf("verified record = %#v", got)
	}
}

func TestVerifyAuthoritativeFormCartRejectsStalePrice(t *testing.T) {
	record := map[string]interface{}{"items": []interface{}{map[string]interface{}{
		"productId": "p1", "quantity": 3, "unitPrice": 19.99, "lineTotal": 59.97,
	}}, "subtotal": 59.97, "total": 59.97}
	_, err := verifyAuthoritativeFormCart(authoritativeOrderForm(), record, map[string]map[string]interface{}{
		"p1": {"_id": "p1", "price": 20.0},
	})
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Status != http.StatusConflict || serviceErr.Message != "FORM_STALE_PRICE" {
		t.Fatalf("error = %#v", err)
	}
}

func TestVerifyAuthoritativeFormCartRejectsMissingProductAndPrice(t *testing.T) {
	record := map[string]interface{}{"items": []interface{}{map[string]interface{}{"productId": "p1", "quantity": 1, "unitPrice": 10, "lineTotal": 10}}, "subtotal": 10, "total": 10}
	_, err := verifyAuthoritativeFormCart(authoritativeOrderForm(), record, nil)
	if serviceErr, ok := err.(*ServiceError); !ok || serviceErr.Message != "FORM_PRODUCT_NOT_FOUND" {
		t.Fatalf("missing product error = %#v", err)
	}
	_, err = verifyAuthoritativeFormCart(authoritativeOrderForm(), record, map[string]map[string]interface{}{"p1": {"_id": "p1"}})
	if serviceErr, ok := err.(*ServiceError); !ok || serviceErr.Message != "FORM_PRODUCT_PRICE_MISSING" {
		t.Fatalf("missing price error = %#v", err)
	}
}

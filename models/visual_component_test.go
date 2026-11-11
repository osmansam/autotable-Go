package models

import (
	"encoding/json"
	"go.mongodb.org/mongo-driver/bson"
	"reflect"
	"testing"
)

func TestVisualComponentKeepsOpaqueJSONThroughBSON(t *testing.T) {
	raw := json.RawMessage(`{"document":{"version":2,"id":"doc","name":"Card","root":{"id":"root","type":"section","children":[]}},"theme":{"palettes":{"light":{"primary-foreground":"#fff"}}},"ProductSKU":"Original"}`)
	input := VisualComponentDesign{Name: " Card ", Value: raw}
	if err := ValidateVisualComponentDesign(&input); err != nil {
		t.Fatal(err)
	}
	if input.Name != "Card" {
		t.Fatal("name not trimmed")
	}
	encoded, err := bson.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var output VisualComponentDesign
	if err = bson.Unmarshal(encoded, &output); err != nil {
		t.Fatal(err)
	}
	var left, right any
	_ = json.Unmarshal(raw, &left)
	_ = json.Unmarshal(output.Value, &right)
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("authored JSON changed: %s", output.Value)
	}
}
func TestVisualComponentRejectsInvalidEnvelope(t *testing.T) {
	for _, raw := range []string{`null`, `{}`, `{"document":{"version":99}}`, `{"document":{"version":2,"id":"doc","root":{"id":"root","type":"text","children":[]}}}`} {
		value := VisualComponentDesign{Name: "Card", Value: json.RawMessage(raw)}
		if ValidateVisualComponentDesign(&value) == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

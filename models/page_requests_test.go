package models

import (
	"encoding/json"
	"go.mongodb.org/mongo-driver/bson"
	"testing"
)

func requestPage(t *testing.T, input string) PageModel {
	t.Helper()
	var page PageModel
	if err := json.Unmarshal([]byte(input), &page); err != nil {
		t.Fatal(err)
	}
	return page
}

func TestPageRequestsSurviveJSONBSONRoundTrip(t *testing.T) {
	page := requestPage(t, `{"name":"Products","requests":[{"id":"products","name":"Products","source":{"transport":"table-source","kind":"schema","schemaName":"products"},"defaults":{"filters":{"active":false,"price":0,"category":null},"controls":{"page":1,"limit":20,"asc":false}},"result":{"kind":"records"}}],"sections":[{"type":"component","component":{"id":"table","type":"table","requestBindings":{"data":{"requestId":"products","overrides":{"controls":{"sort":"name"}}}}}}]}`)
	bytes, err := bson.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	var restored PageModel
	if err := bson.Unmarshal(bytes, &restored); err != nil {
		t.Fatal(err)
	}
	bytes, err = json.Marshal(restored)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]interface{}
	if err := json.Unmarshal(bytes, &value); err != nil {
		t.Fatal(err)
	}
	requests, ok := value["requests"].([]interface{})
	if !ok || len(requests) != 1 {
		t.Fatalf("requests lost: %s", bytes)
	}
	filters := requests[0].(map[string]interface{})["defaults"].(map[string]interface{})["filters"].(map[string]interface{})
	if filters["active"] != false || filters["price"] != float64(0) {
		t.Fatalf("typed defaults lost: %v", filters)
	}
	if category, ok := filters["category"]; !ok || category != nil {
		t.Fatalf("null lost: %v", filters)
	}
	component := value["sections"].([]interface{})[0].(map[string]interface{})["component"].(map[string]interface{})
	if component["requestBindings"] == nil {
		t.Fatalf("connection lost: %s", bytes)
	}
}

func TestPageRequestsValidateReferencesAndSources(t *testing.T) {
	for _, tc := range []struct{ name, input string }{
		{"missing schema", `{"name":"P","requests":[{"id":"r","name":"R","source":{"transport":"table-source","kind":"schema"},"result":{"kind":"records"}}]}`},
		{"duplicate request", `{"name":"P","requests":[{"id":"r","name":"R","source":{"transport":"list","schemaName":"s"},"result":{"kind":"records"}},{"id":"r","name":"R","source":{"transport":"list","schemaName":"s"},"result":{"kind":"records"}}]}`},
		{"missing request", `{"name":"P","sections":[{"type":"component","component":{"id":"c","type":"table","requestBindings":{"data":{"requestId":"missing"}}}}]}`},
		{"unsafe selector", `{"name":"P","requests":[{"id":"r","name":"R","source":{"transport":"list","schemaName":"s"},"result":{"kind":"records","path":["__proto__"]}}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page := requestPage(t, tc.input)
			if err := ValidatePageRuntimeConfig(&page); err == nil {
				t.Fatal("invalid page request was accepted")
			}
		})
	}
}

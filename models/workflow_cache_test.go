package models

import (
	"encoding/json"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestWorkflowCacheConfigurationSurvivesJSONAndBSON(t *testing.T) {
	var workflow DynamicWorkflow
	if err := json.Unmarshal([]byte(`{"name":"loadPublicPage","isRedisCached":true,"cacheTime":5}`), &workflow); err != nil {
		t.Fatal(err)
	}
	raw, err := bson.Marshal(workflow)
	if err != nil {
		t.Fatal(err)
	}
	var restored DynamicWorkflow
	if err := bson.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if !restored.IsRedisCached || restored.CacheTime != 5 {
		t.Fatalf("cache settings lost: %#v", restored)
	}
	var old DynamicWorkflow
	if err := json.Unmarshal([]byte(`{"name":"existing"}`), &old); err != nil {
		t.Fatal(err)
	}
	if old.IsRedisCached {
		t.Fatal("existing workflow unexpectedly opted into caching")
	}
}

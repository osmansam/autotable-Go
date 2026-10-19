package utils

import (
	"encoding/json"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestReadOnlyPipelineRejectsUnsafeStagesAndCollectionReferences(t *testing.T) {
	for _, input := range []string{
		`[{"$out":"target"}]`, `[{"$merge":"target"}]`,
		`[{"$lookup":{"from":"other_project_records","as":"records"}}]`,
		`[{"$facet":{"nested":[{"$out":"target"}]}}]`,
		`[{"$project":{"x":{"$function":{"body":"function(){return 1}","args":[],"lang":"js"}}}}]`,
		`[{"$match":{},"$limit":1}]`,
	} {
		if _, err := ReadOnlyProjectPipeline(models.PipelineStage{PipelineJSON: input}, "t", "p"); err == nil {
			t.Errorf("accepted unsafe pipeline %s", input)
		}
	}
	if _, err := ReadOnlyProjectPipeline(models.PipelineStage{PipelineJSON: `[{"$lookup":{"from":"tenant_t_project_p_products","as":"products"}}]`}, "t", "p"); err != nil {
		t.Fatalf("same-project read-only lookup rejected: %v", err)
	}
}

func TestPipelineQueryValuesRemainLiteral(t *testing.T) {
	value := `text with "quotes" and \\slashes`
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		result := ReplacePlaceholdersWithQueryParams(`[{"$match":{"name":"{{name}}"}}]`, c)
		var stages []map[string]map[string]interface{}
		if err := json.Unmarshal([]byte(result), &stages); err != nil {
			t.Fatalf("query text corrupted pipeline JSON: %v", err)
		}
		if len(stages) != 1 || stages[0]["$match"]["name"] != value {
			t.Fatalf("query changed pipeline structure/value: %s", result)
		}
		return nil
	})
	if _, err := app.Test(httptest.NewRequest("GET", "/?name="+url.QueryEscape(value), nil)); err != nil {
		t.Fatal(err)
	}
}

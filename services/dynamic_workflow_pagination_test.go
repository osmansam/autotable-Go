package services

import (
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/utils"
)

func TestWorkflowFindRecordsWindowUsesTableSourcePager(t *testing.T) {
	pagination := &workflowTableSourcePagination{
		Pager: utils.Pager{
			Enabled: true,
			Page:    9,
			Limit:   50,
			Skip:    400,
		},
	}

	limit, skip, err := workflowFindRecordsWindow(map[string]interface{}{"limit": 500}, pagination, false)
	if err != nil {
		t.Fatalf("workflowFindRecordsWindow() error = %v", err)
	}
	if limit != 50 || skip != 400 {
		t.Fatalf("workflowFindRecordsWindow() = (%d, %d), want (50, 400)", limit, skip)
	}
}

func TestWorkflowTableSourceResponseUsesDatabasePaginationMetadata(t *testing.T) {
	items := make([]map[string]interface{}, 50)
	for index := range items {
		items[index] = map[string]interface{}{"index": index + 400}
	}
	pagination := &workflowTableSourcePagination{
		Applied: true,
		Pager: utils.Pager{
			Enabled:    true,
			Page:       9,
			Limit:      50,
			Skip:       400,
			TotalItems: 1270,
			TotalPages: 26,
		},
	}

	got := workflowTableSourceResponse(items, utils.Pager{
		Enabled: true,
		Page:    9,
		Limit:   50,
		Skip:    400,
	}, pagination)

	gotItems, ok := got["items"].([]map[string]interface{})
	if !ok || len(gotItems) != 50 {
		t.Fatalf("items = %#v, want 50 pre-paginated rows", got["items"])
	}
	if got["totalItems"] != int64(1270) || got["totalPages"] != 26 || got["currentPage"] != 9 {
		t.Fatalf("pagination = %#v", fiber.Map{
			"totalItems":  got["totalItems"],
			"totalPages":  got["totalPages"],
			"currentPage": got["currentPage"],
		})
	}
}

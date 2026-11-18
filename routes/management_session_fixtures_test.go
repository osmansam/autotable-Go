package routes

import (
	"context"
	"github.com/osmansam/autotableGo/middlewares"
	"github.com/osmansam/autotableGo/models"
	"testing"
)

func allowManagementSessions(t *testing.T) {
	t.Helper()
	t.Cleanup(middlewares.OverrideManagementSessionValidator(func(context.Context, models.ManagementPrincipal) error { return nil }))
}

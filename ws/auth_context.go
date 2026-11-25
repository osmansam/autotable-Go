package ws

import (
	"errors"
	"github.com/gofiber/websocket/v2"
)

const AuthenticatedContextKey = "authenticatedWebsocketContext"

type AuthenticatedContext struct {
	TenantID, ProjectID, TenantSlug, ProjectSlug string
}

// Authentication context comes exclusively from verified route cookies. Legacy
// query/header context may mirror it, but can never select another event stream.
func authenticatedContext(c *websocket.Conn) (AuthenticatedContext, error) {
	auth, ok := c.Locals(AuthenticatedContextKey).(AuthenticatedContext)
	if !ok || auth.TenantID == "" || auth.ProjectID == "" || auth.TenantSlug == "" || auth.ProjectSlug == "" {
		return auth, errors.New("authenticated websocket context required")
	}
	checks := []struct {
		query, header string
		allowed       []string
	}{
		{"tenantSlug", "X-Tenant-Slug", []string{auth.TenantSlug}},
		{"projectSlug", "X-Project-Slug", []string{auth.ProjectSlug}},
		{"tenantId", "X-Tenant-Id", []string{auth.TenantID, auth.TenantSlug}},
		{"projectId", "X-Project-Id", []string{auth.ProjectID, auth.ProjectSlug}},
	}
	for _, check := range checks {
		for _, value := range []string{c.Query(check.query), c.Headers(check.header)} {
			if value == "" {
				continue
			}
			match := false
			for _, allowed := range check.allowed {
				if value == allowed {
					match = true
				}
			}
			if !match {
				return auth, errors.New("websocket context conflicts with authenticated project")
			}
		}
	}
	return auth, nil
}

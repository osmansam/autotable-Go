package services

import (
	"fmt"
	"github.com/osmansam/autotableGo/models"
	"net/http"
	"time"
)

// All application-configured outbound APIs use the same DNS-pinned transport
// and redirect policy. A caller's timeout also bounds DNS checks on redirects.
func (s *DynamicService) protectedHTTPClient(timeout time.Duration, credential *models.ExternalAPICredential) (*http.Client, func()) {
	transport := s.workflowHTTPTransport
	cleanup := func() {}
	if transport == nil {
		owned := newWorkflowHTTPTransport(nil, nil)
		transport = owned
		cleanup = owned.CloseIdleConnections
	}
	return &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 || !workflowCallAPIRedirectHostAllowed(req.URL.Hostname(), credential) {
			return fmt.Errorf("api redirect target host is not allowed")
		}
		return validateWorkflowCallAPIURL(req.Context(), req.URL.String())
	}}, cleanup
}

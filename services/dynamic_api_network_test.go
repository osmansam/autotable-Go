package services

import (
	"context"
	"github.com/osmansam/autotableGo/models"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestDynamicAPIRejectsPrivateNetworkTargets(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; _, _ = w.Write([]byte(`{"ok":true}`)) }))
	defer server.Close()
	for _, target := range []string{server.URL, "http://localhost/internal", "http://169.254.169.254/metadata", "file:///tmp/file"} {
		service := &DynamicService{}
		_, err := service.ExecuteDynamicAPI(context.Background(), ExecuteDynamicAPIInput{TenantID: "t", ProjectID: "p", Schema: "records", APIName: "test", Container: &models.ContainerModel{SchemaName: "records", DynamicApis: []models.DynamicApiModel{{Name: "test", Method: "GET", Url: target, IsActive: true}}}})
		if err == nil {
			t.Errorf("accepted unsafe target %s", target)
		}
	}
	if calls != 0 {
		t.Fatalf("private target contacted %d times", calls)
	}
}

// Exercise request encoding against a local server while the configured URL stays
// public. Production always constructs the validating network dialer.
func dynamicAPILocalTestTransport(target string) http.RoundTripper {
	parsed, _ := url.Parse(target)
	return workflowHTTPAuditTransport(func(request *http.Request) (*http.Response, error) {
		clone := request.Clone(request.Context())
		address := *clone.URL
		address.Host = parsed.Host
		address.Scheme = parsed.Scheme
		clone.URL = &address
		return http.DefaultTransport.RoundTrip(clone)
	})
}

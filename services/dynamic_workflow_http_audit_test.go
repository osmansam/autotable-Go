package services

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"golang.org/x/net/dns/dnsmessage"
)

type workflowHTTPAuditTransport func(*http.Request) (*http.Response, error)

func (fn workflowHTTPAuditTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func workflowHTTPAuditUseTransport(t *testing.T, transport http.RoundTripper) *DynamicService {
	t.Helper()
	t.Setenv("WORKFLOW_CALL_API_ALLOWED_DOMAINS", "")
	return &DynamicService{workflowHTTPTransport: transport}
}

func workflowHTTPAuditStep(config map[string]interface{}) models.DynamicWorkflowStep {
	if config == nil {
		config = map[string]interface{}{}
	}
	if _, ok := config["url"]; !ok {
		config["url"] = "https://8.8.8.8/endpoint"
	}
	return models.DynamicWorkflowStep{Type: models.WorkflowStepTypeCallAPI, TimeoutSec: 1, Config: config}
}

func workflowHTTPAuditResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}
}

// Return deterministic DNS answers without depending on the network or machine hosts file.
func workflowHTTPAuditUseDNS(t *testing.T, address [4]byte) {
	t.Helper()
	original := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			var size [2]byte
			if _, err := io.ReadFull(server, size[:]); err != nil {
				return
			}
			packet := make([]byte, binary.BigEndian.Uint16(size[:]))
			if _, err := io.ReadFull(server, packet); err != nil {
				return
			}
			var query dnsmessage.Message
			if query.Unpack(packet) != nil {
				return
			}
			answer := dnsmessage.Message{
				Header:    dnsmessage.Header{ID: query.ID, Response: true, RecursionAvailable: true},
				Questions: query.Questions,
			}
			for _, question := range query.Questions {
				if question.Type == dnsmessage.TypeA {
					answer.Answers = append(answer.Answers, dnsmessage.Resource{
						Header: dnsmessage.ResourceHeader{Name: question.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET},
						Body:   &dnsmessage.AResource{A: address},
					})
				}
			}
			packet, err := answer.Pack()
			if err != nil {
				return
			}
			binary.BigEndian.PutUint16(size[:], uint16(len(packet)))
			_, _ = server.Write(append(size[:], packet...))
		}()
		return client, nil
	}}
	t.Cleanup(func() { net.DefaultResolver = original })
}

func TestWorkflowHTTPAuditStopsRedirectLoops(t *testing.T) {
	calls := 0
	service := workflowHTTPAuditUseTransport(t, workflowHTTPAuditTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls > 12 {
			return workflowHTTPAuditResponse(req, http.StatusOK, `{"unexpected":"loop completed"}`), nil
		}
		response := workflowHTTPAuditResponse(req, http.StatusFound, "redirect")
		response.Header.Set("Location", fmt.Sprintf("https://8.8.8.8/redirect/%d", calls))
		return response, nil
	}))
	output, err := service.workflowCallAPI(context.Background(), workflowHTTPAuditStep(nil), workflowExecutionPayload{})
	if err == nil || output != nil {
		t.Fatalf("redirect loop returned output %#v and error %v; want an error", output, err)
	}
	if calls > 10 {
		t.Fatalf("redirect loop made %d requests; want at most 10", calls)
	}
}

func TestWorkflowHTTPAuditRedirectRejectsPrivateDNSAnswer(t *testing.T) {
	workflowHTTPAuditUseDNS(t, [4]byte{127, 0, 0, 1})
	calls := 0
	service := workflowHTTPAuditUseTransport(t, workflowHTTPAuditTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			response := workflowHTTPAuditResponse(req, http.StatusFound, "redirect")
			response.Header.Set("Location", "https://private.workflow.invalid/secret")
			return response, nil
		}
		return workflowHTTPAuditResponse(req, http.StatusOK, `{"leaked":true}`), nil
	}))
	output, err := service.workflowCallAPI(context.Background(), workflowHTTPAuditStep(nil), workflowExecutionPayload{})
	if err == nil || output != nil || calls != 1 {
		t.Fatalf("private DNS redirect = %#v, error %v, requests %d; want rejection before second request", output, err, calls)
	}
}

func workflowHTTPAuditUseServer(t *testing.T, handler http.Handler) *DynamicService {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
	}
	t.Cleanup(transport.CloseIdleConnections)
	return workflowHTTPAuditUseTransport(t, transport)
}

func TestWorkflowHTTPAuditRendersAndSendsRequest(t *testing.T) {
	service := workflowHTTPAuditUseServer(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body interface{}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"path": req.URL.RequestURI(), "method": req.Method, "body": body,
			"contentType": req.Header.Get("Content-Type"), "userHeader": req.Header.Get("X-Custom"),
			"authorization": req.Header.Get("Authorization"),
		})
	}))
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE", "GET"} {
		t.Run(method, func(t *testing.T) {
			step := workflowHTTPAuditStep(map[string]interface{}{
				"url":     "http://8.8.8.8/orders/{{record.id}}?from={{vars.source}}",
				"method":  " " + strings.ToLower(method) + " ",
				"body":    map[string]interface{}{"quantity": "{{record.quantity}}", "label": "Order {{record.id}}"},
				"headers": map[string]interface{}{"X-Custom": "visible", "aUtHoRiZaTiOn": "Bearer untrusted"},
			})
			payload := workflowExecutionPayload{
				Record:    map[string]interface{}{"id": "order-7", "quantity": 3},
				Variables: map[string]interface{}{"source": "queue"},
			}
			output, err := service.workflowCallAPI(context.Background(), step, payload)
			if err != nil {
				t.Fatalf("workflowCallAPI() error = %v", err)
			}
			contentType := ""
			if method == "POST" || method == "PUT" || method == "PATCH" {
				contentType = "application/json"
			}
			want := map[string]interface{}{
				"path": "/orders/order-7?from=queue", "method": method,
				"body":        map[string]interface{}{"quantity": float64(3), "label": "Order order-7"},
				"contentType": contentType, "userHeader": "visible", "authorization": "",
			}
			if !reflect.DeepEqual(output, want) {
				t.Fatalf("request echoed %#v, want %#v", output, want)
			}
		})
	}
}

func TestWorkflowHTTPAuditDecodesSuccessfulResponses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   interface{}
	}{
		{"created object", http.StatusCreated, `{"id":7}`, map[string]interface{}{"id": float64(7)}},
		{"array", http.StatusOK, `[true,"ok",2]`, []interface{}{true, "ok", float64(2)}},
		{"JSON string", http.StatusOK, `"accepted"`, "accepted"},
		{"JSON null", http.StatusOK, `null`, nil},
		{"empty response", http.StatusNoContent, "", ""},
		{"plain text", http.StatusOK, "accepted\n", "accepted\n"},
		{"malformed JSON", http.StatusOK, `{"incomplete":`, `{"incomplete":`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := workflowHTTPAuditUseTransport(t, workflowHTTPAuditTransport(func(req *http.Request) (*http.Response, error) {
				return workflowHTTPAuditResponse(req, tt.status, tt.body), nil
			}))
			output, err := service.workflowCallAPI(context.Background(), workflowHTTPAuditStep(nil), workflowExecutionPayload{})
			if err != nil || !reflect.DeepEqual(output, tt.want) {
				t.Fatalf("workflowCallAPI() = %#v, %v; want %#v", output, err, tt.want)
			}
		})
	}
}

func TestWorkflowHTTPAuditRejectsUnsuccessfulStatus(t *testing.T) {
	for _, status := range []int{199, 300, 400, 401, 429, 500, 599} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			service := workflowHTTPAuditUseTransport(t, workflowHTTPAuditTransport(func(req *http.Request) (*http.Response, error) {
				return workflowHTTPAuditResponse(req, status, "remote failure: "+strings.Repeat("x", 2000)), nil
			}))
			output, err := service.workflowCallAPI(context.Background(), workflowHTTPAuditStep(nil), workflowExecutionPayload{})
			if output != nil || err == nil || !strings.Contains(err.Error(), fmt.Sprintf("status %d", status)) || !strings.Contains(err.Error(), "remote failure:") {
				t.Fatalf("workflowCallAPI() = %#v, %v; want bounded status failure", output, err)
			}
			if len(err.Error()) > 1100 || !strings.Contains(err.Error(), "truncated") {
				t.Fatalf("status error was not bounded: %d bytes", len(err.Error()))
			}
		})
	}
}

func TestWorkflowHTTPAuditTimeoutAndCancellationReachServer(t *testing.T) {
	for _, cancelParent := range []bool{false, true} {
		t.Run(fmt.Sprintf("parent cancellation=%v", cancelParent), func(t *testing.T) {
			started := make(chan struct{})
			stopped := make(chan struct{})
			service := workflowHTTPAuditUseServer(t, http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
				close(started)
				<-req.Context().Done()
				close(stopped)
			}))
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			wantErr := context.DeadlineExceeded
			if cancelParent {
				wantErr = context.Canceled
				go func() {
					select {
					case <-started:
						cancel()
					case <-ctx.Done():
					}
				}()
			}
			step := workflowHTTPAuditStep(map[string]interface{}{"url": "http://8.8.8.8/slow"})
			before := time.Now()
			output, err := service.processWorkflowStepWithTimeout(ctx, step, &workflowExecutionPayload{})
			if output != nil || !errors.Is(err, wantErr) {
				t.Fatalf("workflow call = %#v, %v; want %v", output, err, wantErr)
			}
			if !cancelParent && time.Since(before) >= 2500*time.Millisecond {
				t.Fatal("request exceeded its one-second workflow timeout")
			}
			select {
			case <-stopped:
			case <-time.After(time.Second):
				t.Fatal("server did not observe request cancellation")
			}
		})
	}
}

func TestWorkflowHTTPAuditAlreadyCancelledContextDoesNotSend(t *testing.T) {
	var calls atomic.Int32
	service := workflowHTTPAuditUseServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := service.workflowCallAPI(ctx, workflowHTTPAuditStep(map[string]interface{}{"url": "http://8.8.8.8/"}), workflowExecutionPayload{})
	if !errors.Is(err, context.Canceled) || calls.Load() != 0 {
		t.Fatalf("cancelled request error = %v, server calls = %d", err, calls.Load())
	}
}

type workflowHTTPAuditBody struct {
	io.Reader
	read   int
	closed bool
}

func (body *workflowHTTPAuditBody) Read(dst []byte) (int, error) {
	n, err := body.Reader.Read(dst)
	body.read += n
	return n, err
}

func (body *workflowHTTPAuditBody) Close() error { body.closed = true; return nil }

type workflowHTTPAuditErrorReader struct{ err error }

func (reader workflowHTTPAuditErrorReader) Read([]byte) (int, error) { return 0, reader.err }

func TestWorkflowHTTPAuditClosesAndBoundsResponseBodies(t *testing.T) {
	readFailure := errors.New("response stream interrupted")
	for _, scenario := range []string{"success", "HTTP error", "read error", "maximum size", "oversized"} {
		t.Run(scenario, func(t *testing.T) {
			status := http.StatusOK
			body := &workflowHTTPAuditBody{Reader: strings.NewReader(`{"ok":true}`)}
			switch scenario {
			case "HTTP error":
				status = http.StatusBadGateway
			case "read error":
				body.Reader = workflowHTTPAuditErrorReader{readFailure}
			case "maximum size":
				body.Reader = strings.NewReader(strings.Repeat("x", 5<<20))
			case "oversized":
				body.Reader = strings.NewReader(strings.Repeat("x", (5<<20)+1000))
			}
			service := workflowHTTPAuditUseTransport(t, workflowHTTPAuditTransport(func(req *http.Request) (*http.Response, error) {
				response := workflowHTTPAuditResponse(req, status, "")
				response.Body = body
				return response, nil
			}))
			output, err := service.workflowCallAPI(context.Background(), workflowHTTPAuditStep(nil), workflowExecutionPayload{})
			if !body.closed {
				t.Fatal("response body was not closed")
			}
			switch scenario {
			case "success":
				if err != nil || !reflect.DeepEqual(output, map[string]interface{}{"ok": true}) {
					t.Fatalf("response = %#v, %v", output, err)
				}
			case "maximum size":
				text, ok := output.(string)
				if err != nil || !ok || len(text) != 5<<20 {
					t.Fatalf("boundary response type=%T, error=%v", output, err)
				}
			case "read error":
				if output != nil || !errors.Is(err, readFailure) {
					t.Fatalf("read failure = %#v, %v", output, err)
				}
			case "oversized":
				if output != nil || err == nil || !strings.Contains(err.Error(), "exceeds max size") {
					t.Fatalf("oversized response type=%T, error=%v", output, err)
				}
				if body.read > (5<<20)+1 {
					t.Fatalf("read %d bytes from oversized response", body.read)
				}
			case "HTTP error":
				if output != nil || err == nil || !strings.Contains(err.Error(), "status 502") {
					t.Fatalf("HTTP failure = %#v, %v", output, err)
				}
			}
		})
	}
}

func TestWorkflowHTTPAuditProtectedCredentialHeadersWinOnWire(t *testing.T) {
	service := workflowHTTPAuditUseServer(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"auth": req.Header.Get("Authorization"), "key": req.Header.Get("X-API-Key"), "custom": req.Header.Get("X-Custom")})
	}))
	for _, authType := range []string{models.ExternalAPIAuthTypeBearer, models.ExternalAPIAuthTypeHeader} {
		t.Run(authType, func(t *testing.T) {
			headers, err := workflowExternalAPICredentialProtectedHeaders(models.ExternalAPICredential{
				AuthType: authType, HeaderName: "X-API-Key", AllowedDomains: []string{"8.8.8.8"},
			}, "protected-secret", "8.8.8.8", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			body, status, err := utils.ExecuteApiRequestWithClient(context.Background(), &http.Client{Transport: service.workflowHTTPTransport}, "GET", "http://8.8.8.8/", nil,
				map[string]string{"aUtHoRiZaTiOn": "untrusted", "x-api-key": "untrusted-key", "X-Custom": "visible"}, headers)
			if err != nil || status != http.StatusOK {
				t.Fatalf("request status=%d, error=%v", status, err)
			}
			var got map[string]string
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"auth": "Bearer protected-secret", "key": "untrusted-key", "custom": "visible"}
			if authType == models.ExternalAPIAuthTypeHeader {
				want["auth"] = ""
				want["key"] = "protected-secret"
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("headers on wire = %#v, want %#v", got, want)
			}
		})
	}
}

func TestWorkflowHTTPAuditRejectsNonPublicIPv4Destinations(t *testing.T) {
	t.Setenv("WORKFLOW_CALL_API_ALLOWED_DOMAINS", "")
	for _, address := range []string{"0.1.2.3", "100.64.0.1", "100.100.100.200", "100.127.255.254", "240.0.0.1", "255.255.255.255", "::ffff:100.100.100.200"} {
		t.Run(address, func(t *testing.T) {
			rawURL := "https://" + net.JoinHostPort(address, "443") + "/"
			if err := validateWorkflowCallAPIURL(context.Background(), rawURL); err == nil {
				t.Fatalf("nonpublic destination %s was accepted", address)
			}
			if workflowCallAPIRedirectHostAllowed(address, nil) {
				t.Fatalf("redirect accepted nonpublic destination %s", address)
			}
		})
	}
	for _, address := range []string{"8.8.8.8", "1.1.1.1", "100.63.255.255", "100.128.0.1", "2606:4700:4700::1111", "::ffff:8.8.8.8"} {
		if err := validateWorkflowCallAPIURL(context.Background(), "https://"+net.JoinHostPort(address, "443")); err != nil {
			t.Errorf("public destination %s was rejected: %v", address, err)
		}
	}
}

func TestWorkflowHTTPAuditRedirectPolicy(t *testing.T) {
	for _, tt := range []struct {
		name, location, allowed string
		wantError               bool
	}{
		{"relative", "/finished", "8.8.8.8", false},
		{"public allowed destination", "https://1.1.1.1/finished", "8.8.8.8,1.1.1.1", false},
		{"public unlisted destination", "https://1.1.1.1/finished", "8.8.8.8", true},
		{"loopback destination", "http://127.0.0.1/private", "", true},
		{"localhost destination", "http://localhost/private", "", true},
		{"IPv6 loopback", "http://[::1]/private", "", true},
		{"suffix spoof", "https://evil-example.com/private", "8.8.8.8,example.com", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			service := workflowHTTPAuditUseTransport(t, workflowHTTPAuditTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					response := workflowHTTPAuditResponse(req, http.StatusFound, "")
					response.Header.Set("Location", tt.location)
					return response, nil
				}
				return workflowHTTPAuditResponse(req, http.StatusOK, `{"redirected":true}`), nil
			}))
			t.Setenv("WORKFLOW_CALL_API_ALLOWED_DOMAINS", tt.allowed)
			output, err := service.workflowCallAPI(context.Background(), workflowHTTPAuditStep(nil), workflowExecutionPayload{})
			if tt.wantError {
				if err == nil || output != nil || calls != 1 {
					t.Fatalf("blocked redirect = %#v, %v, calls=%d", output, err, calls)
				}
			} else if err != nil || calls != 2 || !reflect.DeepEqual(output, map[string]interface{}{"redirected": true}) {
				t.Fatalf("permitted redirect = %#v, %v, calls=%d", output, err, calls)
			}
		})
	}
}

func TestWorkflowHTTPAuditRejectsInvalidRequestBeforeTransport(t *testing.T) {
	for _, tt := range []struct {
		name      string
		config    map[string]interface{}
		timeout   int
		wantError string
	}{
		{"missing timeout", map[string]interface{}{"url": "https://8.8.8.8"}, 0, "timeoutSec"},
		{"forbidden method", map[string]interface{}{"method": "TRACE"}, 1, "method is not allowed"},
		{"malformed method", map[string]interface{}{"method": "GET\r\nX-Test:bad"}, 1, "method is not allowed"},
		{"blocked host", map[string]interface{}{"url": "https://10.1.2.3"}, 1, "blocked network"},
		{"unsupported scheme", map[string]interface{}{"url": "file:///etc/passwd"}, 1, "scheme"},
		{"malformed URL", map[string]interface{}{"url": "http://[bad-ip"}, 1, "invalid call_api url"},
		{"missing host", map[string]interface{}{"url": "https:///path"}, 1, "host is required"},
		{"invalid credential ID", map[string]interface{}{"credentialId": "invalid"}, 1, "credential not found"},
		{"unserializable body", map[string]interface{}{"body": make(chan int)}, 1, "unsupported type"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			service := workflowHTTPAuditUseTransport(t, workflowHTTPAuditTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				return workflowHTTPAuditResponse(req, http.StatusOK, `{}`), nil
			}))
			step := workflowHTTPAuditStep(tt.config)
			step.TimeoutSec = tt.timeout
			output, err := service.workflowCallAPI(context.Background(), step, workflowExecutionPayload{})
			if output != nil || err == nil || !strings.Contains(err.Error(), tt.wantError) || calls != 0 {
				t.Fatalf("invalid request = %#v, %v, calls=%d; want %q before transport", output, err, calls, tt.wantError)
			}
		})
	}
}

func TestWorkflowHTTPAuditTransportFailurePreservesCause(t *testing.T) {
	cause := errors.New("connection reset")
	service := workflowHTTPAuditUseTransport(t, workflowHTTPAuditTransport(func(*http.Request) (*http.Response, error) { return nil, cause }))
	output, err := service.workflowCallAPI(context.Background(), workflowHTTPAuditStep(nil), workflowExecutionPayload{})
	if output != nil || !errors.Is(err, cause) {
		t.Fatalf("transport failure = %#v, %v", output, err)
	}
}

func TestWorkflowHTTPAuditValidatesDNSAnswersBeforeSending(t *testing.T) {
	for _, tt := range []struct {
		name      string
		address   [4]byte
		wantError bool
	}{
		{"public", [4]byte{8, 8, 8, 8}, false},
		{"private", [4]byte{10, 0, 0, 7}, true},
		{"loopback", [4]byte{127, 0, 0, 1}, true},
		{"metadata", [4]byte{169, 254, 169, 254}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workflowHTTPAuditUseDNS(t, tt.address)
			calls := 0
			service := workflowHTTPAuditUseTransport(t, workflowHTTPAuditTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				return workflowHTTPAuditResponse(req, http.StatusOK, `{"ok":true}`), nil
			}))
			step := workflowHTTPAuditStep(map[string]interface{}{"url": "https://api.workflow.invalid/"})
			output, err := service.workflowCallAPI(context.Background(), step, workflowExecutionPayload{})
			if tt.wantError {
				if output != nil || err == nil || !strings.Contains(err.Error(), "blocked network") || calls != 0 {
					t.Fatalf("blocked DNS = %#v, %v, calls=%d", output, err, calls)
				}
			} else if err != nil || calls != 1 || !reflect.DeepEqual(output, map[string]interface{}{"ok": true}) {
				t.Fatalf("public DNS = %#v, %v, calls=%d", output, err, calls)
			}
		})
	}
}

func TestWorkflowHTTPAuditHostAllowlistsRespectDomainBoundaries(t *testing.T) {
	t.Setenv("WORKFLOW_CALL_API_ALLOWED_DOMAINS", " Example.COM., api.example.net ")
	credential := &models.ExternalAPICredential{AllowedDomains: []string{" EXAMPLE.COM. "}}
	for _, tt := range []struct {
		host               string
		global, credential bool
	}{
		{"example.com", true, true},
		{"API.Example.COM.", true, true},
		{"deep.api.example.com", true, true},
		{"api.example.net", true, false},
		{"evil-example.com", false, false},
		{"example.com.attacker.test", false, false},
		{"example.net", false, false},
		{"", false, false},
	} {
		if got := workflowCallAPIHostAllowed(tt.host); got != tt.global {
			t.Errorf("global allowlist(%q) = %v, want %v", tt.host, got, tt.global)
		}
		if got := workflowCallAPIRedirectHostAllowed(tt.host, credential); got != tt.credential {
			t.Errorf("credential redirect allowlist(%q) = %v, want %v", tt.host, got, tt.credential)
		}
	}
}

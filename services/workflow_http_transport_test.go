package services

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
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

	"golang.org/x/net/dns/dnsmessage"
)

type workflowTransportResolverFunc func(context.Context, string) ([]net.IPAddr, error)

func (fn workflowTransportResolverFunc) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return fn(ctx, host)
}

type workflowTransportDialFunc func(context.Context, string, string) (net.Conn, error)

func (fn workflowTransportDialFunc) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return fn(ctx, network, address)
}

func TestWorkflowHTTPTransportPinsOnlyValidatedAddresses(t *testing.T) {
	t.Setenv("WORKFLOW_CALL_API_ALLOWED_DOMAINS", "api.workflow.invalid")
	connectionFailure := errors.New("first public address unreachable")
	var dialed []string
	resolver := workflowTransportResolverFunc(func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host != "api.workflow.invalid" {
			t.Errorf("resolved host = %q", host)
		}
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("2606:4700:4700::1111")}}, nil
	})
	client, server := net.Pipe()
	defer server.Close()
	transport := newWorkflowHTTPTransport(resolver, workflowTransportDialFunc(func(_ context.Context, _ string, address string) (net.Conn, error) {
		dialed = append(dialed, address)
		if address == "8.8.8.8:8443" {
			return nil, connectionFailure
		}
		if address != "[2606:4700:4700::1111]:8443" {
			return nil, fmt.Errorf("dial received an unresolved or unvalidated address: %s", address)
		}
		return client, nil
	}))
	defer transport.CloseIdleConnections()
	conn, err := transport.DialContext(context.Background(), "tcp", "api.workflow.invalid:8443")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if !reflect.DeepEqual(dialed, []string{"8.8.8.8:8443", "[2606:4700:4700::1111]:8443"}) {
		t.Fatalf("dialed addresses = %v", dialed)
	}
}

func TestWorkflowHTTPTransportRejectsUnsafeDialTargets(t *testing.T) {
	for _, tt := range []struct {
		name       string
		host       string
		allowed    string
		addresses  []net.IPAddr
		resolveErr error
	}{
		{name: "mixed public and private answers", host: "api.workflow.invalid", addresses: []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("10.0.0.1")}}},
		{name: "mapped private IPv6", host: "api.workflow.invalid", addresses: []net.IPAddr{{IP: net.ParseIP("::ffff:127.0.0.1")}}},
		{name: "private literal", host: "169.254.169.254"},
		{name: "localhost", host: "localhost"},
		{name: "outside domain allowlist", host: "api.workflow.invalid", allowed: "other.workflow.invalid", addresses: []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}},
		{name: "no DNS answer", host: "api.workflow.invalid"},
		{name: "invalid DNS answer", host: "api.workflow.invalid", addresses: []net.IPAddr{{}}},
		{name: "malformed DNS address", host: "api.workflow.invalid", addresses: []net.IPAddr{{IP: net.IP{1, 2}}}},
		{name: "DNS failure", host: "api.workflow.invalid", resolveErr: errors.New("DNS unavailable")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("WORKFLOW_CALL_API_ALLOWED_DOMAINS", tt.allowed)
			dials := 0
			resolver := workflowTransportResolverFunc(func(context.Context, string) ([]net.IPAddr, error) { return tt.addresses, tt.resolveErr })
			transport := newWorkflowHTTPTransport(resolver, workflowTransportDialFunc(func(context.Context, string, string) (net.Conn, error) {
				dials++
				return nil, errors.New("unsafe dial was reached")
			}))
			defer transport.CloseIdleConnections()
			conn, err := transport.DialContext(context.Background(), "tcp", net.JoinHostPort(tt.host, "80"))
			if conn != nil || err == nil || dials != 0 {
				t.Fatalf("unsafe target conn=%v, error=%v, dials=%d", conn, err, dials)
			}
			if tt.resolveErr != nil && !errors.Is(err, tt.resolveErr) {
				t.Fatalf("resolution cause lost: %v", err)
			}
		})
	}
}

func TestWorkflowHTTPTransportBoundsPreflightByStepTimeout(t *testing.T) {
	t.Setenv("WORKFLOW_CALL_API_ALLOWED_DOMAINS", "")
	original := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	t.Cleanup(func() { net.DefaultResolver = original })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	start := time.Now()
	step := workflowHTTPAuditStep(map[string]interface{}{"url": "http://unresponsive.workflow.invalid/"})
	output, err := (&DynamicService{}).workflowCallAPI(ctx, step, workflowExecutionPayload{})
	if output != nil || err == nil || time.Since(start) >= 2500*time.Millisecond {
		t.Fatalf("preflight output=%v, error=%v, elapsed=%v; want deadline within one-second step timeout", output, err, time.Since(start))
	}
}

func TestWorkflowHTTPTransportCancellationReachesResolutionAndDial(t *testing.T) {
	t.Setenv("WORKFLOW_CALL_API_ALLOWED_DOMAINS", "")
	for _, duringResolution := range []bool{true, false} {
		t.Run(fmt.Sprintf("during resolution=%v", duringResolution), func(t *testing.T) {
			resolver := workflowTransportResolverFunc(func(ctx context.Context, _ string) ([]net.IPAddr, error) {
				if duringResolution {
					<-ctx.Done()
					return nil, ctx.Err()
				}
				return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
			})
			transport := newWorkflowHTTPTransport(resolver, workflowTransportDialFunc(func(ctx context.Context, _, _ string) (net.Conn, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}))
			defer transport.CloseIdleConnections()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			conn, err := transport.DialContext(ctx, "tcp", "api.workflow.invalid:443")
			if conn != nil || !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("cancelled dial conn=%v, error=%v", conn, err)
			}
		})
	}
}

func TestWorkflowHTTPTransportDoesNotDelegateToEnvironmentProxy(t *testing.T) {
	t.Setenv("WORKFLOW_CALL_API_ALLOWED_DOMAINS", "")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:3128")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:3128")
	t.Setenv("ALL_PROXY", "socks5://127.0.0.1:1080")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.WriteString(w, req.Host)
	}))
	defer server.Close()
	var dialed string
	transport := newWorkflowHTTPTransport(nil, workflowTransportDialFunc(func(ctx context.Context, network, address string) (net.Conn, error) {
		dialed = address
		if address != "8.8.8.8:80" {
			return nil, fmt.Errorf("unexpected proxy or unresolved destination %s", address)
		}
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}))
	defer transport.CloseIdleConnections()
	response, err := (&http.Client{Transport: transport}).Get("http://8.8.8.8/proxy-check")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "8.8.8.8" || dialed != "8.8.8.8:80" {
		t.Fatalf("proxy check body=%q, error=%v, dialed=%q", body, err, dialed)
	}
}

func TestWorkflowHTTPTransportPreservesTLSHostnameAndVerification(t *testing.T) {
	t.Setenv("WORKFLOW_CALL_API_ALLOWED_DOMAINS", "")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.WriteString(w, req.Host+"|"+req.TLS.ServerName)
	}))
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	resolver := workflowTransportResolverFunc(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	})
	transport := newWorkflowHTTPTransport(resolver, workflowTransportDialFunc(func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "8.8.8.8:443" {
			return nil, fmt.Errorf("TLS dial received %q", address)
		}
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}))
	defer transport.CloseIdleConnections()
	transport.TLSClientConfig = &tls.Config{RootCAs: roots}
	client := &http.Client{Transport: transport}
	response, err := client.Get("https://example.com/tls-check")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || string(body) != "example.com|example.com" {
		t.Fatalf("TLS host/SNI response = %q, error = %v", body, err)
	}
	response, err = client.Get("https://wrong.workflow.invalid/tls-check")
	if response != nil {
		_ = response.Body.Close()
	}
	var hostnameError x509.HostnameError
	if err == nil || !errors.As(err, &hostnameError) {
		t.Fatalf("mismatched certificate error = %v; want hostname verification failure", err)
	}
}

// The first lookup passes preflight; the next answer models a rebinding attack.
// Both DNS and the old transport's network connection remain in memory.
func TestWorkflowHTTPTransportRejectsRebindingAfterPreflight(t *testing.T) {
	t.Setenv("WORKFLOW_CALL_API_ALLOWED_DOMAINS", "")
	var lookups atomic.Int32
	resolver := workflowTransportDNSResolver(func() [4]byte {
		if lookups.Add(1) == 1 {
			return [4]byte{8, 8, 8, 8}
		}
		return [4]byte{127, 0, 0, 1}
	})
	originalResolver := net.DefaultResolver
	net.DefaultResolver = resolver
	t.Cleanup(func() { net.DefaultResolver = originalResolver })
	var reachedPrivate atomic.Bool
	oldTransport := &http.Transport{DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		addresses, err := resolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, address := range addresses {
			if address.IP.IsLoopback() {
				reachedPrivate.Store(true)
			}
		}
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			request, err := http.ReadRequest(bufio.NewReader(server))
			if err != nil {
				return
			}
			_ = request.Body.Close()
			_, _ = io.WriteString(server, "HTTP/1.1 200 OK\r\nContent-Length: 15\r\nConnection: close\r\n\r\n{\"leaked\":true}")
		}()
		return client, nil
	}}
	t.Cleanup(oldTransport.CloseIdleConnections)
	originalTransport := http.DefaultTransport
	http.DefaultTransport = oldTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })

	step := workflowHTTPAuditStep(map[string]interface{}{"url": "http://rebind.workflow.invalid/secret"})
	output, err := (&DynamicService{}).workflowCallAPI(context.Background(), step, workflowExecutionPayload{})
	if output != nil || err == nil || !strings.Contains(err.Error(), "blocked network") || reachedPrivate.Load() {
		t.Fatalf("rebinding result = %#v, error = %v, private connection = %v; want rejection before private dial", output, err, reachedPrivate.Load())
	}
	if lookups.Load() < 2 {
		t.Fatal("regression did not exercise separate preflight and connection lookups")
	}
}

func workflowTransportDNSResolver(answer func() [4]byte) *net.Resolver {
	return &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
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
			response := dnsmessage.Message{
				Header:    dnsmessage.Header{ID: query.ID, Response: true, RecursionAvailable: true},
				Questions: query.Questions,
			}
			for _, question := range query.Questions {
				if question.Type == dnsmessage.TypeA {
					response.Answers = append(response.Answers, dnsmessage.Resource{
						Header: dnsmessage.ResourceHeader{Name: question.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET},
						Body:   &dnsmessage.AResource{A: answer()},
					})
				}
			}
			packet, err := response.Pack()
			if err != nil {
				return
			}
			binary.BigEndian.PutUint16(size[:], uint16(len(packet)))
			_, _ = server.Write(append(size[:], packet...))
		}()
		return client, nil
	}}
}

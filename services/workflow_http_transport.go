package services

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

type workflowIPResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type workflowNetworkDialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

// newWorkflowHTTPTransport keeps target resolution inside the dial operation.
// A validated hostname is never handed to the underlying dialer for resolution.
func newWorkflowHTTPTransport(resolver workflowIPResolver, dialer workflowNetworkDialer) *http.Transport {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if dialer == nil {
		dialer = &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	}
	return &http.Transport{
		// Proxies can resolve the destination themselves, bypassing address pinning.
		Proxy:                 nil,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: time.Second,
		IdleConnTimeout:       30 * time.Second,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   2,
		MaxConnsPerHost:       2,
		ForceAttemptHTTP2:     true,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("invalid call_api dial address: %w", err)
			}
			addresses, err := workflowCallAPIAddresses(ctx, host, resolver)
			if err != nil {
				return nil, err
			}
			var dialErr error
			for _, target := range addresses {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(target.String(), port))
				if err == nil {
					return conn, nil
				}
				dialErr = err
			}
			return nil, dialErr
		},
	}
}

func workflowCallAPIAddresses(ctx context.Context, host string, resolver workflowIPResolver) ([]net.IPAddr, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !workflowCallAPIRedirectHostAllowed(host, nil) {
		return nil, fmt.Errorf("call_api url host is not allowed or resolves to a blocked network")
	}
	if ip := net.ParseIP(host); ip != nil {
		return []net.IPAddr{{IP: ip}}, nil
	}
	addresses, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve call_api host: %w", err)
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("call_api host did not resolve")
	}
	for _, address := range addresses {
		if address.IP.To16() == nil || workflowBlockedOutboundIP(address.IP) {
			return nil, fmt.Errorf("call_api url resolves to a blocked network")
		}
	}
	return addresses, nil
}

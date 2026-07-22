package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxExecuteAPIResponseBytes = 5 << 20

var errAPIRedirectTargetHostNotAllowed = errors.New("api redirect target host is not allowed")

// ExecuteApiRequest makes an HTTP request based on the provided method, URL, body, and context.
// It returns the response body as a byte slice and any error encountered.
func ExecuteApiRequest(ctx context.Context, method string, url string, body interface{}) ([]byte, error) {
	respBody, _, err := ExecuteApiRequestWithStatus(ctx, method, url, body)
	return respBody, err
}

// ExecuteApiRequestWithStatus makes an HTTP request and returns the response body and status code.
func ExecuteApiRequestWithStatus(ctx context.Context, method string, url string, body interface{}) ([]byte, int, error) {
	return ExecuteApiRequestWithStatusAndHeaders(ctx, method, url, body, nil, nil, nil)
}

// ExecuteApiRequestWithStatusAndHeaders makes an HTTP request with user-provided
// headers and protected headers. Protected headers always win.
func ExecuteApiRequestWithStatusAndHeaders(ctx context.Context, method string, requestURL string, body interface{}, userHeaders, protectedHeaders map[string]string, hostAllowed func(string) bool) ([]byte, int, error) {
	var reqBody []byte
	var err error

	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	if hostAllowed != nil {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if !hostAllowed(req.URL.Hostname()) {
				return errAPIRedirectTargetHostNotAllowed
			}
			return nil
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, 0, err
	}

	if method == "POST" || method == "PATCH" || method == "PUT" {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range userHeaders {
		if protectedHeaderName(key) {
			continue
		}
		req.Header.Set(key, value)
	}
	for key, value := range protectedHeaders {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, errAPIRedirectTargetHostNotAllowed) {
			return nil, 0, errAPIRedirectTargetHostNotAllowed
		}
		return nil, 0, err
	}
	defer resp.Body.Close()

	limitedReader := io.LimitReader(resp.Body, maxExecuteAPIResponseBytes+1)
	respBody, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if len(respBody) > maxExecuteAPIResponseBytes {
		return nil, resp.StatusCode, fmt.Errorf("api response exceeds max size of %d bytes", maxExecuteAPIResponseBytes)
	}

	return respBody, resp.StatusCode, nil
}

func HostnameFromURL(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	return parsed.Hostname(), nil
}

func protectedHeaderName(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), "Authorization")
}

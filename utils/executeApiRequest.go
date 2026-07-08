package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const maxExecuteAPIResponseBytes = 5 << 20

// ExecuteApiRequest makes an HTTP request based on the provided method, URL, body, and context.
// It returns the response body as a byte slice and any error encountered.
func ExecuteApiRequest(ctx context.Context, method string, url string, body interface{}) ([]byte, error) {
	var reqBody []byte
	var err error

	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	if method == "POST" || method == "PATCH" || method == "PUT" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	limitedReader := io.LimitReader(resp.Body, maxExecuteAPIResponseBytes+1)
	respBody, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, err
	}
	if len(respBody) > maxExecuteAPIResponseBytes {
		return nil, fmt.Errorf("api response exceeds max size of %d bytes", maxExecuteAPIResponseBytes)
	}

	return respBody, nil
}

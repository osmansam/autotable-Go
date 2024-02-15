package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"
)

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

    respBody, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        return nil, err
    }

    return respBody, nil
}

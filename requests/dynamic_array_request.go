package requests

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type ArrayMutationOperation string

const (
	ArrayMutationAdd    ArrayMutationOperation = "add"
	ArrayMutationUpdate ArrayMutationOperation = "update"
)

type ArrayRowMutationRequest struct {
	RowIdentityField string                 `json:"rowIdentityField"`
	Item             map[string]interface{} `json:"item,omitempty"`
	Updates          map[string]interface{} `json:"updates,omitempty"`
}

type ArrayReorderRequest struct {
	RowIdentityField string        `json:"rowIdentityField"`
	OrderField       string        `json:"orderField"`
	RowIdentities    []interface{} `json:"rowIdentities"`
}

func decodeStrictJSON(reader io.Reader, destination interface{}) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("request body must contain one JSON value")
		}
		return err
	}
	return nil
}

func ParseArrayRowMutation(reader io.Reader, operation ArrayMutationOperation) (ArrayRowMutationRequest, error) {
	var request ArrayRowMutationRequest
	if err := decodeStrictJSON(reader, &request); err != nil {
		return request, err
	}
	request.RowIdentityField = strings.TrimSpace(request.RowIdentityField)
	if request.RowIdentityField == "" {
		return request, fmt.Errorf("rowIdentityField is required")
	}
	switch operation {
	case ArrayMutationAdd:
		if len(request.Item) == 0 {
			return request, fmt.Errorf("item is required")
		}
		if request.Updates != nil {
			return request, fmt.Errorf("updates is not allowed for add")
		}
	case ArrayMutationUpdate:
		if len(request.Updates) == 0 {
			return request, fmt.Errorf("updates is required")
		}
		if request.Item != nil {
			return request, fmt.Errorf("item is not allowed for update")
		}
	default:
		return request, fmt.Errorf("unsupported array mutation operation %q", operation)
	}
	return request, nil
}

func arrayIdentityKey(value interface{}) (string, error) {
	switch value.(type) {
	case string, float64, bool:
		return fmt.Sprintf("%T:%v", value, value), nil
	default:
		return "", fmt.Errorf("row identities must be strings, numbers, or booleans")
	}
}

func ParseArrayReorder(reader io.Reader) (ArrayReorderRequest, error) {
	var request ArrayReorderRequest
	if err := decodeStrictJSON(reader, &request); err != nil {
		return request, err
	}
	request.RowIdentityField = strings.TrimSpace(request.RowIdentityField)
	request.OrderField = strings.TrimSpace(request.OrderField)
	if request.RowIdentityField == "" {
		return request, fmt.Errorf("rowIdentityField is required")
	}
	if request.OrderField == "" {
		return request, fmt.Errorf("orderField is required")
	}
	if len(request.RowIdentities) == 0 {
		return request, fmt.Errorf("rowIdentities is required")
	}
	seen := make(map[string]struct{}, len(request.RowIdentities))
	for _, identity := range request.RowIdentities {
		key, err := arrayIdentityKey(identity)
		if err != nil {
			return request, err
		}
		if _, exists := seen[key]; exists {
			return request, fmt.Errorf("rowIdentities contains duplicate value")
		}
		seen[key] = struct{}{}
	}
	return request, nil
}

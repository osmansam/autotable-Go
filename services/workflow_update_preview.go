package services

import (
	"bytes"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// applyWorkflowUpdateForAccess previews supported Mongo updates before final-record
// authorization. Unsupported modifiers and selectors fail closed for protected records.
func applyWorkflowUpdateForAccess(document map[string]interface{}, update bson.M) error {
	// Reject conflicting or unsupported paths before simulating any mutation.
	paths := make([]string, 0)
	for operator, rawFields := range update {
		switch operator {
		case "$set", "$unset", "$inc", "$addToSet", "$push", "$pull", "$pullAll", "$setOnInsert":
		default:
			return fmt.Errorf("workflow update operator is not allowed: %s", operator)
		}
		fields, ok := workflowMapConfig(rawFields)
		if !ok {
			return fmt.Errorf("workflow update operator %s requires an object value", operator)
		}
		for path := range fields {
			if path != strings.TrimSpace(path) {
				return fmt.Errorf("workflow update path cannot contain surrounding whitespace: %s", path)
			}
			for _, part := range strings.Split(path, ".") {
				if part == "" || strings.HasPrefix(part, "$") || strings.ContainsRune(part, '\x00') {
					return fmt.Errorf("invalid workflow target path: %s", path)
				}
			}
			for _, previous := range paths {
				if path == previous || strings.HasPrefix(path, previous+".") || strings.HasPrefix(previous, path+".") {
					return fmt.Errorf("workflow update has conflicting paths: %s and %s", previous, path)
				}
			}
			paths = append(paths, path)
		}
	}
	for operator, rawFields := range update {
		fields, ok := workflowMapConfig(rawFields)
		if !ok {
			return fmt.Errorf("workflow update operator %s requires an object value", operator)
		}
		for path, value := range fields {
			switch operator {
			case "$set":
				if err := workflowSetPath(document, path, cloneWorkflowSnapshot(value)); err != nil {
					return err
				}
			case "$unset":
				if err := workflowUnsetPath(document, path); err != nil {
					return err
				}
			case "$inc":
				current, exists := workflowPathValue(document, path)
				incremented, err := workflowMutationIncrement(current, value, exists)
				if err != nil {
					return fmt.Errorf("workflow update $inc for %s: %w", path, err)
				}
				if err := workflowSetPath(document, path, incremented); err != nil {
					return err
				}
			case "$addToSet", "$push":
				current, exists := workflowPathValue(document, path)
				items, isArray := workflowMutationArray(current)
				if exists && !isArray {
					return fmt.Errorf("workflow update %s requires an array target for %s", operator, path)
				}
				items = append([]interface{}(nil), items...)
				values := []interface{}{value}
				if valueMap, ok := workflowMapConfig(value); ok {
					if each, exists := valueMap["$each"]; exists {
						expanded, ok := workflowMutationArray(each)
						if !ok || len(valueMap) != 1 {
							return fmt.Errorf("workflow update %s supports only an array $each modifier for %s", operator, path)
						}
						values = expanded
					} else {
						for key := range valueMap {
							if strings.HasPrefix(key, "$") {
								return fmt.Errorf("workflow update %s modifier %s is not supported", operator, key)
							}
						}
					}
				} else if ordered, ok := value.(bson.D); ok {
					for _, field := range ordered {
						if strings.HasPrefix(field.Key, "$") {
							return fmt.Errorf("workflow update %s ordered modifiers are not supported", operator)
						}
					}
				} else {
					valueType, _, err := bson.MarshalValue(value)
					if err != nil {
						return fmt.Errorf("workflow update %s operand cannot be encoded safely: %w", operator, err)
					}
					if valueType == bsontype.EmbeddedDocument {
						return fmt.Errorf("workflow update %s encoded document modifiers are not supported for %s", operator, path)
					}
				}
				for _, candidate := range values {
					if operator == "$addToSet" {
						contains, err := workflowMutationSliceContains(items, candidate)
						if err != nil {
							return err
						}
						if contains {
							continue
						}
					}
					items = append(items, cloneWorkflowSnapshot(candidate))
				}
				if items == nil {
					items = []interface{}{}
				}
				if err := workflowSetPath(document, path, items); err != nil {
					return err
				}
			case "$pull":
				valueType, _, err := bson.MarshalValue(value)
				if err != nil {
					return fmt.Errorf("workflow update $pull operand cannot be encoded safely: %w", err)
				}
				if valueType == bsontype.EmbeddedDocument || valueType == bsontype.Regex {
					return fmt.Errorf("workflow update $pull document and regex selectors are not supported for %s", path)
				}
				current, exists := workflowPathValue(document, path)
				if !exists {
					continue
				}
				items, isArray := workflowMutationArray(current)
				if !isArray {
					return fmt.Errorf("workflow update $pull requires an array target for %s", path)
				}
				kept := make([]interface{}, 0, len(items))
				for _, candidate := range items {
					equal, err := workflowMutationValuesEqual(candidate, value)
					if err != nil {
						return err
					}
					if !equal {
						kept = append(kept, candidate)
					}
				}
				if err := workflowSetPath(document, path, kept); err != nil {
					return err
				}
			case "$pullAll":
				remove, ok := workflowMutationArray(value)
				if !ok {
					return fmt.Errorf("workflow update $pullAll requires an array for %s", path)
				}
				current, exists := workflowPathValue(document, path)
				if !exists {
					continue
				}
				items, isArray := workflowMutationArray(current)
				if !isArray {
					return fmt.Errorf("workflow update $pullAll requires an array target for %s", path)
				}
				kept := make([]interface{}, 0, len(items))
				for _, candidate := range items {
					contains, err := workflowMutationSliceContains(remove, candidate)
					if err != nil {
						return err
					}
					if !contains {
						kept = append(kept, candidate)
					}
				}
				if err := workflowSetPath(document, path, kept); err != nil {
					return err
				}
			case "$setOnInsert":
				// Existing candidates are never affected by $setOnInsert.
			default:
				return fmt.Errorf("workflow update operator is not allowed: %s", operator)
			}
		}
	}
	return nil
}

func workflowMutationIncrement(current, increment interface{}, exists bool) (interface{}, error) {
	right := reflect.ValueOf(increment)
	if !right.IsValid() || (right.Kind() != reflect.Int && right.Kind() != reflect.Int32 && right.Kind() != reflect.Int64 && right.Kind() != reflect.Float32 && right.Kind() != reflect.Float64) {
		return nil, fmt.Errorf("requires finite numeric values")
	}
	if _, ok := workflowMutationNumber(increment); !ok {
		return nil, fmt.Errorf("requires finite numeric values")
	}
	if !exists {
		return increment, nil
	}
	if _, ok := workflowMutationNumber(current); !ok {
		return nil, fmt.Errorf("requires finite numeric values")
	}
	left := reflect.ValueOf(current)
	if !left.IsValid() || (left.Kind() != reflect.Int && left.Kind() != reflect.Int32 && left.Kind() != reflect.Int64 && left.Kind() != reflect.Float32 && left.Kind() != reflect.Float64) {
		return nil, fmt.Errorf("requires finite numeric values")
	}
	if left.Kind() != reflect.Float32 && left.Kind() != reflect.Float64 && right.Kind() != reflect.Float32 && right.Kind() != reflect.Float64 {
		leftInteger, rightInteger := left.Int(), right.Int()
		if (rightInteger > 0 && leftInteger > math.MaxInt64-rightInteger) || (rightInteger < 0 && leftInteger < math.MinInt64-rightInteger) {
			return nil, fmt.Errorf("integer overflow cannot be simulated safely")
		}
		sum := leftInteger + rightInteger
		if left.Kind() != reflect.Int64 && right.Kind() != reflect.Int64 && sum >= math.MinInt32 && sum <= math.MaxInt32 {
			return int32(sum), nil
		}
		return sum, nil
	}
	leftNumber, _ := workflowNumber(current)
	rightNumber, _ := workflowNumber(increment)
	sum := leftNumber + rightNumber
	if math.IsNaN(sum) || math.IsInf(sum, 0) {
		return nil, fmt.Errorf("requires finite numeric values")
	}
	return sum, nil
}

func workflowMutationNumber(value interface{}) (*big.Rat, bool) {
	switch typed := value.(type) {
	case int:
		return new(big.Rat).SetInt64(int64(typed)), true
	case int32:
		return new(big.Rat).SetInt64(int64(typed)), true
	case int64:
		return new(big.Rat).SetInt64(typed), true
	case float32:
		return workflowMutationNumber(float64(typed))
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return nil, false
		}
		return new(big.Rat).SetFloat64(typed), true
	case primitive.Decimal128:
		return new(big.Rat).SetString(typed.String())
	default:
		return nil, false
	}
}

func workflowMutationValuesEqual(left, right interface{}) (bool, error) {
	for _, value := range []*interface{}{&left, &right} {
		reflected := reflect.ValueOf(*value)
		if reflected.IsValid() && (reflected.Kind() == reflect.Map || reflected.Kind() == reflect.Slice) && reflected.IsNil() {
			*value = nil
		}
	}
	leftNumber, leftNumeric := workflowMutationNumber(left)
	rightNumber, rightNumeric := workflowMutationNumber(right)
	if leftNumeric || rightNumeric {
		return leftNumeric && rightNumeric && leftNumber.Cmp(rightNumber) == 0, nil
	}
	if _, ordered := left.(bson.D); ordered {
		return false, fmt.Errorf("workflow array ordered document comparisons are not supported")
	}
	if _, ordered := right.(bson.D); ordered {
		return false, fmt.Errorf("workflow array ordered document comparisons are not supported")
	}
	leftMap, leftObject := workflowMapConfig(left)
	rightMap, rightObject := workflowMapConfig(right)
	if leftObject || rightObject {
		if !leftObject || !rightObject || len(leftMap) != len(rightMap) {
			return false, nil
		}
		for key, value := range leftMap {
			other, exists := rightMap[key]
			if !exists {
				return false, nil
			}
			equal, err := workflowMutationValuesEqual(value, other)
			if err != nil || !equal {
				return false, err
			}
		}
		if len(leftMap) > 1 {
			return false, fmt.Errorf("workflow array document equality requires preserved BSON field order")
		}
		return true, nil
	}
	leftItems, leftArray := workflowMutationArray(left)
	rightItems, rightArray := workflowMutationArray(right)
	if leftArray || rightArray {
		if !leftArray || !rightArray || len(leftItems) != len(rightItems) {
			return false, nil
		}
		for index, item := range leftItems {
			equal, err := workflowMutationValuesEqual(item, rightItems[index])
			if err != nil || !equal {
				return false, err
			}
		}
		return true, nil
	}
	leftType, leftBytes, err := bson.MarshalValue(left)
	if err != nil {
		return false, fmt.Errorf("workflow array value cannot be compared safely: %w", err)
	}
	rightType, rightBytes, err := bson.MarshalValue(right)
	if err != nil {
		return false, fmt.Errorf("workflow array value cannot be compared safely: %w", err)
	}
	return leftType == rightType && bytes.Equal(leftBytes, rightBytes), nil
}

func workflowMutationSliceContains(items []interface{}, wanted interface{}) (bool, error) {
	for _, item := range items {
		equal, err := workflowMutationValuesEqual(item, wanted)
		if err != nil || equal {
			return equal, err
		}
	}
	return false, nil
}

func workflowMutationArray(value interface{}) ([]interface{}, bool) {
	if _, document := value.(bson.D); document {
		return nil, false
	}
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() || (reflected.Kind() != reflect.Slice && reflected.Kind() != reflect.Array) ||
		(reflected.Kind() == reflect.Slice && reflected.IsNil()) || reflected.Type().Elem().Kind() == reflect.Uint8 {
		return nil, false
	}
	return workflowInterfaceSlice(value)
}

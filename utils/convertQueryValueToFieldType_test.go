package utils

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestConvertQueryValueToFieldType_Integer(t *testing.T) {
	tests := []struct {
		name       string
		fieldName  string
		fieldType  string
		queryValue string
		wantValue  int
		wantError  bool
	}{
		{
			name:       "Clean integer",
			fieldName:  "age",
			fieldType:  "int",
			queryValue: "123",
			wantValue:  123,
			wantError:  false,
		},
		{
			name:       "Integer with leading space",
			fieldName:  "age",
			fieldType:  "int",
			queryValue: " 123",
			wantValue:  123,
			wantError:  false,
		},
		{
			name:       "Integer with trailing space",
			fieldName:  "age",
			fieldType:  "int",
			queryValue: "123 ",
			wantValue:  123,
			wantError:  false,
		},
		{
			name:       "Integer with spaces on both sides",
			fieldName:  "age",
			fieldType:  "int",
			queryValue: " 123 ",
			wantValue:  123,
			wantError:  false,
		},
		{
			name:       "Integer with double quotes",
			fieldName:  "age",
			fieldType:  "int",
			queryValue: "\"123\"",
			wantValue:  123,
			wantError:  false,
		},
		{
			name:       "Integer with single quotes",
			fieldName:  "age",
			fieldType:  "int",
			queryValue: "'123'",
			wantValue:  123,
			wantError:  false,
		},
		{
			name:       "Integer with spaces and quotes",
			fieldName:  "age",
			fieldType:  "int",
			queryValue: " \"123\" ",
			wantValue:  123,
			wantError:  false,
		},
		{
			name:       "Single digit integer",
			fieldName:  "age",
			fieldType:  "int",
			queryValue: "6",
			wantValue:  6,
			wantError:  false,
		},
		{
			name:       "Single digit with spaces",
			fieldName:  "age",
			fieldType:  "int",
			queryValue: " 6 ",
			wantValue:  6,
			wantError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ConvertQueryValueToFieldType(tt.fieldName, tt.fieldType, tt.queryValue)
			
			if (err != nil) != tt.wantError {
				t.Errorf("ConvertQueryValueToFieldType() error = %v, wantError %v", err, tt.wantError)
				return
			}
			
			if !tt.wantError {
				gotInt, ok := got.(int)
				if !ok {
					t.Errorf("ConvertQueryValueToFieldType() returned type %T, want int", got)
					return
				}
				
				if gotInt != tt.wantValue {
					t.Errorf("ConvertQueryValueToFieldType() = %v, want %v", gotInt, tt.wantValue)
				}
			}
		})
	}
}

func TestConvertQueryValueToFieldType_IntegerList(t *testing.T) {
	tests := []struct {
		name       string
		fieldName  string
		fieldType  string
		queryValue string
		wantValues []int
		wantError  bool
	}{
		{
			name:       "Clean integer list",
			fieldName:  "ages",
			fieldType:  "int",
			queryValue: "10,20,30",
			wantValues: []int{10, 20, 30},
			wantError:  false,
		},
		{
			name:       "Integer list with spaces",
			fieldName:  "ages",
			fieldType:  "int",
			queryValue: " 10 , 20 , 30 ",
			wantValues: []int{10, 20, 30},
			wantError:  false,
		},
		{
			name:       "Integer list with quotes",
			fieldName:  "ages",
			fieldType:  "int",
			queryValue: "\"10,20,30\"",
			wantValues: []int{10, 20, 30},
			wantError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ConvertQueryValueToFieldType(tt.fieldName, tt.fieldType, tt.queryValue)
			
			if (err != nil) != tt.wantError {
				t.Errorf("ConvertQueryValueToFieldType() error = %v, wantError %v", err, tt.wantError)
				return
			}
			
			if !tt.wantError {
				gotBson, ok := got.(bson.M)
				if !ok {
					t.Errorf("ConvertQueryValueToFieldType() returned type %T, want bson.M", got)
					return
				}
				
				gotIn, ok := gotBson["$in"]
				if !ok {
					t.Errorf("ConvertQueryValueToFieldType() missing $in operator")
					return
				}
				
				gotIntSlice, ok := gotIn.([]int)
				if !ok {
					t.Errorf("ConvertQueryValueToFieldType() $in value type %T, want []int", gotIn)
					return
				}
				
				if len(gotIntSlice) != len(tt.wantValues) {
					t.Errorf("ConvertQueryValueToFieldType() length = %v, want %v", len(gotIntSlice), len(tt.wantValues))
					return
				}
				
				for i, v := range gotIntSlice {
					if v != tt.wantValues[i] {
						t.Errorf("ConvertQueryValueToFieldType() [%d] = %v, want %v", i, v, tt.wantValues[i])
					}
				}
			}
		})
	}
}

package utils

import (
	"testing"
)

func TestEvaluateEquation(t *testing.T) {
	tests := []struct {
		name     string
		equation string
		data     map[string]interface{}
		want     float64
		wantErr  bool
	}{
		{
			name:     "Simple addition",
			equation: "x + y",
			data:     map[string]interface{}{"x": 10, "y": 5},
			want:     15,
			wantErr:  false,
		},
		{
			name:     "Multiplication and parentheses",
			equation: "(x + y) * 2",
			data:     map[string]interface{}{"x": 10, "y": 5},
			want:     30,
			wantErr:  false,
		},
		{
			name:     "Division",
			equation: "x / y",
			data:     map[string]interface{}{"x": 10, "y": 2},
			want:     5,
			wantErr:  false,
		},
		{
			name:     "Division by zero",
			equation: "x / 0",
			data:     map[string]interface{}{"x": 10},
			want:     0,
			wantErr:  true,
		},
		{
			name:     "Variable not found",
			equation: "x + z",
			data:     map[string]interface{}{"x": 10},
			want:     0,
			wantErr:  true,
		},
		{
			name:     "Complex expression",
			equation: "x * 2 + y / 2 - 5",
			data:     map[string]interface{}{"x": 10, "y": 10},
			want:     20, // 20 + 5 - 5
			wantErr:  false,
		},
        {
			name:     "Float values",
			equation: "x * y",
			data:     map[string]interface{}{"x": 1.5, "y": 2.0},
			want:     3.0,
			wantErr:  false,
		},
        {
            name:     "Update simulation",
            equation: "(x + y) * 2",
            data: func() map[string]interface{} {
                // Simulate existing item
                existing := map[string]interface{}{"x": 10, "y": 5, "z": 30}
                // Simulate update: change x to 20
                update := map[string]interface{}{"x": 20}
                // Merge
                for k, v := range update {
                    existing[k] = v
                }
                return existing
            }(),
            want:    50, // (20 + 5) * 2
            wantErr: false,
        },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvaluateEquation(tt.equation, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvaluateEquation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if gotFloat, ok := got.(float64); ok {
					if gotFloat != tt.want {
						t.Errorf("EvaluateEquation() = %v, want %v", got, tt.want)
					}
				} else {
					t.Errorf("EvaluateEquation() returned non-float64: %T", got)
				}
			}
		})
	}
}

package utils

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
)

// EvaluateEquation parses and evaluates a mathematical expression string
// using values from the provided data map.
func EvaluateEquation(equation string, data map[string]interface{}) (interface{}, error) {
	if equation == "" {
		return nil, nil
	}

	// Parse the expression
	expr, err := parser.ParseExpr(equation)
	if err != nil {
		return nil, fmt.Errorf("failed to parse equation: %w", err)
	}

	// Evaluate the AST
	result, err := evalAST(expr, data)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func evalAST(node ast.Node, data map[string]interface{}) (float64, error) {
	switch n := node.(type) {
	case *ast.BinaryExpr:
		left, err := evalAST(n.X, data)
		if err != nil {
			return 0, err
		}
		right, err := evalAST(n.Y, data)
		if err != nil {
			return 0, err
		}

		switch n.Op {
		case token.ADD:
			return left + right, nil
		case token.SUB:
			return left - right, nil
		case token.MUL:
			return left * right, nil
		case token.QUO:
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			return left / right, nil
		default:
			return 0, fmt.Errorf("unsupported operator: %v", n.Op)
		}

	case *ast.ParenExpr:
		return evalAST(n.X, data)

	case *ast.BasicLit:
		if n.Kind == token.INT || n.Kind == token.FLOAT {
			val, err := strconv.ParseFloat(n.Value, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid number literal: %v", n.Value)
			}
			return val, nil
		}
		return 0, fmt.Errorf("unsupported literal type: %v", n.Kind)

	case *ast.Ident:
		val, ok := data[n.Name]
		if !ok {
			return 0, fmt.Errorf("variable not found: %s", n.Name)
		}
		return toFloat(val)

	default:
		return 0, fmt.Errorf("unsupported expression node: %T", node)
	}
}

func toFloat(val interface{}) (float64, error) {
	switch v := val.(type) {
	case int:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", val)
	}
}

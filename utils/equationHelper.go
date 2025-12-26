package utils

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// EquationContext holds the context needed for equation evaluation
type EquationContext struct {
	TenantID  string
	ProjectID string
	Data      map[string]interface{}
}

// EvaluateEquation parses and evaluates a mathematical expression string
// using values from the provided data map and supporting object field references.
func EvaluateEquation(equation string, data map[string]interface{}) (interface{}, error) {
	// For backward compatibility, create default context
	ctx := &EquationContext{
		Data: data,
	}
	return EvaluateEquationWithContext(equation, ctx)
}

// EvaluateEquationWithContext parses and evaluates a mathematical expression string
// using values from the provided context, supporting both direct variables and object field references.
func EvaluateEquationWithContext(equation string, ctx *EquationContext) (interface{}, error) {
	if equation == "" {
		return nil, nil
	}

	// Parse the expression
	expr, err := parser.ParseExpr(equation)
	if err != nil {
		return nil, fmt.Errorf("failed to parse equation: %w", err)
	}

	// Evaluate the AST
	result, err := evalAST(expr, ctx)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func evalAST(node ast.Node, ctx *EquationContext) (float64, error) {
	switch n := node.(type) {
	case *ast.BinaryExpr:
		left, err := evalAST(n.X, ctx)
		if err != nil {
			return 0, err
		}
		right, err := evalAST(n.Y, ctx)
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
		return evalAST(n.X, ctx)

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
		val, ok := ctx.Data[n.Name]
		if !ok {
			return 0, fmt.Errorf("variable not found: %s", n.Name)
		}
		return toFloat(val)

	case *ast.SelectorExpr:
		// Handle object.field references (e.g., can.x, konu.y)
		return evalObjectFieldReference(n, ctx)

	default:
		return 0, fmt.Errorf("unsupported expression node: %T", node)
	}
}

// evalObjectFieldReference handles object.field references like menu.price or category.quantity
func evalObjectFieldReference(sel *ast.SelectorExpr, ctx *EquationContext) (float64, error) {
	// Extract object name and field name
	objIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return 0, fmt.Errorf("complex object expressions not supported")
	}
	
	objectName := objIdent.Name
	fieldName := sel.Sel.Name
	
	// Check if the object exists in current data
	objectData, exists := ctx.Data[objectName]
	if !exists {
		return 0, fmt.Errorf("object reference %s not found in data", objectName)
	}
	
	// Handle the case where object is already populated (contains the full object data)
	if objectMap, ok := objectData.(map[string]interface{}); ok {
		if fieldValue, fieldExists := objectMap[fieldName]; fieldExists {
			return toFloat(fieldValue)
		}
		// If field doesn't exist in the populated object, return error
		return 0, fmt.Errorf("field %s not found in populated object %s", fieldName, objectName)
	}
	
	// If object is not populated, treat it as ObjectID and fetch from database
	if ctx.TenantID == "" || ctx.ProjectID == "" {
		return 0, fmt.Errorf("cannot resolve object reference %s.%s: missing tenant/project context", objectName, fieldName)
	}
	
	// Convert to ObjectID
	var objectID primitive.ObjectID
	switch v := objectData.(type) {
	case primitive.ObjectID:
		objectID = v
	case string:
		var err error
		objectID, err = primitive.ObjectIDFromHex(v)
		if err != nil {
			return 0, fmt.Errorf("invalid object ID format for %s: %v", objectName, err)
		}
	default:
		return 0, fmt.Errorf("object reference %s is neither populated object nor valid ObjectID (type: %T)", objectName, v)
	}
	
	// Fetch the object from database and extract the field
	return fetchObjectField(objectID, fieldName, objectName, ctx)
}

// fetchObjectField retrieves an object by ID and extracts the specified field value
func fetchObjectField(objectID primitive.ObjectID, fieldName, objectName string, ctx *EquationContext) (float64, error) {
	// We need to find which schema this object belongs to
	// This is a limitation - we need to know the schema name
	// For now, we'll assume the object name matches the schema name
	schemaName := objectName
	
	// Get the collection for this schema
	collection := GetDynamicCollectionForProject(ctx.TenantID, ctx.ProjectID, schemaName)
	
	// Find the object by ID
	dbCtx := context.Background()
	var result map[string]interface{}
	err := collection.FindOne(dbCtx, bson.M{"_id": objectID}).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return 0, fmt.Errorf("object %s with ID %s not found", objectName, objectID.Hex())
		}
		return 0, fmt.Errorf("error fetching object %s: %v", objectName, err)
	}
	
	// Extract the field value
	fieldValue, exists := result[fieldName]
	if !exists {
		return 0, fmt.Errorf("field %s not found in object %s", fieldName, objectName)
	}
	
	return toFloat(fieldValue)
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

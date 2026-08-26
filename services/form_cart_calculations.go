package services

import (
	"fmt"
	"math"
	"math/big"
	"strconv"

	"github.com/osmansam/autotableGo/models"
)

func EvaluateFormCart(form models.FormComponentConfig, record map[string]interface{}) (map[string]interface{}, error) {
	next := cloneWorkflowMap(record)
	for _, list := range form.ObjectLists {
		rawItems, ok := workflowSlice(next[list.Key])
		if !ok {
			rawItems = []interface{}{}
		}
		calculated := make([]interface{}, 0, len(rawItems))
		for index, rawItem := range rawItems {
			item, ok := rawItem.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("FORM_INVALID_QUANTITY: item %d must be an object", index)
			}
			itemCopy := cloneWorkflowMap(item)
			for _, calculation := range list.ItemCalculations {
				if len(calculation.Inputs) != 2 {
					return nil, fmt.Errorf("FORM_CONFIG_NOT_FOUND: unsupported item calculation")
				}
				left, err := formCartNumber(itemCopy[calculation.Inputs[0]])
				if err != nil {
					return nil, fmt.Errorf("FORM_PRODUCT_PRICE_MISSING: item %d field %s", index, calculation.Inputs[0])
				}
				right, err := formCartNumber(itemCopy[calculation.Inputs[1]])
				if err != nil || right <= 0 || calculation.Inputs[1] == "quantity" && right <= 0 {
					return nil, fmt.Errorf("FORM_INVALID_QUANTITY: item %d field %s", index, calculation.Inputs[1])
				}
				precision := formCartPrecision(calculation.Precision)
				switch calculation.Operation {
				case "multiply":
					itemCopy[calculation.TargetField] = formCartMultiply(left, right, precision)
				case "quantityDiscount":
					if calculation.OriginalTargetField == "" || !formCartQuantityDiscountConfigured(calculation) {
						return nil, fmt.Errorf("FORM_CONFIG_NOT_FOUND: invalid quantity discount calculation")
					}
					original := formCartMultiply(left, right, precision)
					itemCopy[calculation.OriginalTargetField] = original
					itemCopy[calculation.TargetField] = original
					if percentage, qualified := formCartDiscountPercentage(calculation, right); qualified {
						itemCopy[calculation.TargetField] = formCartApplyPercentageDiscount(original, percentage, precision)
					}
				default:
					return nil, fmt.Errorf("FORM_CONFIG_NOT_FOUND: unsupported item calculation")
				}
			}
			calculated = append(calculated, itemCopy)
		}
		next[list.Key] = calculated
	}

	for _, summary := range form.Summaries {
		precision := 2
		if summary.Format != nil {
			precision = formCartPrecision(summary.Format.Precision)
		}
		switch summary.Operation {
		case "sum":
			items, _ := workflowSlice(next[summary.ObjectListKey])
			total := new(big.Rat)
			for index, rawItem := range items {
				item, ok := rawItem.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("FORM_INVALID_QUANTITY: item %d must be an object", index)
				}
				value, err := formCartRat(item[summary.SourceField])
				if err != nil {
					return nil, fmt.Errorf("FORM_PRODUCT_PRICE_MISSING: item %d field %s", index, summary.SourceField)
				}
				total.Add(total, value)
			}
			next[summary.TargetField] = formCartRoundRat(total, precision)
		case "copy":
			value, err := formCartRat(next[summary.SourceField])
			if err != nil {
				return nil, fmt.Errorf("FORM_CONFIG_NOT_FOUND: missing summary %s", summary.SourceField)
			}
			next[summary.TargetField] = formCartRoundRat(value, precision)
		default:
			return nil, fmt.Errorf("FORM_CONFIG_NOT_FOUND: unsupported summary calculation")
		}
	}
	return next, nil
}

func formCartQuantityDiscountConfigured(calculation models.FormItemCalculationConfig) bool {
	if len(calculation.DiscountTiers) == 0 {
		return calculation.MinimumQuantity != nil && calculation.DiscountPercentage != nil &&
			*calculation.MinimumQuantity > 0 && *calculation.DiscountPercentage > 0 && *calculation.DiscountPercentage <= 100
	}
	for _, tier := range calculation.DiscountTiers {
		if tier.MinimumQuantity == nil || tier.DiscountPercentage == nil ||
			*tier.MinimumQuantity <= 0 || *tier.DiscountPercentage <= 0 || *tier.DiscountPercentage > 100 {
			return false
		}
	}
	return true
}

func formCartDiscountPercentage(calculation models.FormItemCalculationConfig, quantity float64) (float64, bool) {
	if len(calculation.DiscountTiers) == 0 {
		if quantity >= *calculation.MinimumQuantity {
			return *calculation.DiscountPercentage, true
		}
		return 0, false
	}
	percentage, qualified := float64(0), false
	for _, tier := range calculation.DiscountTiers {
		if quantity < *tier.MinimumQuantity {
			break
		}
		percentage, qualified = *tier.DiscountPercentage, true
	}
	return percentage, qualified
}

func formCartNumber(value interface{}) (float64, error) {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0, fmt.Errorf("not finite")
		}
		return typed, nil
	case float32:
		return float64(typed), nil
	case int:
		return float64(typed), nil
	case int32:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	case string:
		return strconv.ParseFloat(typed, 64)
	default:
		return 0, fmt.Errorf("not numeric")
	}
}

func formCartRat(value interface{}) (*big.Rat, error) {
	number, err := formCartNumber(value)
	if err != nil {
		return nil, err
	}
	rat := new(big.Rat)
	if _, ok := rat.SetString(strconv.FormatFloat(number, 'f', -1, 64)); !ok {
		return nil, fmt.Errorf("invalid decimal")
	}
	return rat, nil
}

func formCartMultiply(left, right float64, precision int) float64 {
	leftRat, _ := formCartRat(left)
	rightRat, _ := formCartRat(right)
	return formCartRoundRat(new(big.Rat).Mul(leftRat, rightRat), precision)
}

func formCartApplyPercentageDiscount(original, percentage float64, precision int) float64 {
	originalRat, _ := formCartRat(original)
	percentageRat, _ := formCartRat(percentage)
	factor := new(big.Rat).Sub(big.NewRat(1, 1), new(big.Rat).Quo(percentageRat, big.NewRat(100, 1)))
	return formCartRoundRat(new(big.Rat).Mul(originalRat, factor), precision)
}

func formCartRoundRat(value *big.Rat, precision int) float64 {
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(precision)), nil)
	scaledNumerator := new(big.Int).Mul(value.Num(), scale)
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(scaledNumerator, value.Denom(), remainder)
	doubleRemainder := new(big.Int).Abs(new(big.Int).Mul(remainder, big.NewInt(2)))
	if doubleRemainder.Cmp(value.Denom()) >= 0 {
		if scaledNumerator.Sign() >= 0 {
			quotient.Add(quotient, big.NewInt(1))
		} else {
			quotient.Sub(quotient, big.NewInt(1))
		}
	}
	result, _ := new(big.Rat).SetFrac(quotient, scale).Float64()
	return result
}

func formCartPrecision(value *int) int {
	if value == nil {
		return 2
	}
	return *value
}

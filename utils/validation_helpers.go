package utils

import (
	"strings"
)

// isValidCreditCard validates a credit card number using the Luhn algorithm
func isValidCreditCard(number string) bool {
	// Remove any spaces or dashes
	number = strings.ReplaceAll(strings.ReplaceAll(number, " ", ""), "-", "")
	
	// Check if it's all digits and has valid length (13-19 digits)
	if len(number) < 13 || len(number) > 19 {
		return false
	}
	
	for _, c := range number {
		if c < '0' || c > '9' {
			return false
		}
	}
	
	// Luhn algorithm
	sum := 0
	isSecond := false
	
	// Traverse from right to left
	for i := len(number) - 1; i >= 0; i-- {
		digit := int(number[i] - '0')
		
		if isSecond {
			digit = digit * 2
			if digit > 9 {
				digit = digit - 9
			}
		}
		
		sum += digit
		isSecond = !isSecond
	}
	
	return sum%10 == 0
}

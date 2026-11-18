package models

import "strings"

func NormalizeManagementEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

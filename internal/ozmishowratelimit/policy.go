// Package ozmishowratelimit exposes a fixed-policy service API, never Redis commands.
package ozmishowratelimit

import "time"

const Path = "/internal/ozmishow/rate-limit"

type Policy struct {
	Max    int
	Window time.Duration
}

type Result struct {
	Allowed           bool  `json:"allowed"`
	RetryAfterSeconds int64 `json:"retryAfterSeconds"`
}

func DefaultPolicies() map[string]Policy {
	return map[string]Policy{
		"login": {10, 5 * time.Minute}, "admin": {600, time.Minute}, "peer": {300, time.Minute},
		"operation:translate": {3, 10 * time.Minute}, "operation:publish": {30, time.Minute},
		"maintenance:prewarm": {2, time.Minute}, "maintenance:revalidate": {30, time.Minute}, "prewarm-host": {1, time.Minute},
	}
}

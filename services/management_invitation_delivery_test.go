package services

import (
	"testing"
	"time"
)

func TestCollaborationDeliveryRetryBudget(t *testing.T) {
	if got := invitationRetryDelay(1); got != 10*time.Second {
		t.Fatalf("first retry=%v", got)
	}
	for _, attempt := range []int{10, 20, 100} {
		if got := invitationRetryDelay(attempt); got != time.Hour {
			t.Fatalf("retry cap=%v", got)
		}
	}
}

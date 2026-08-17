package repositories

import (
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
)

func TestClassifyRefreshRotationRecognizesGraceConflictAndReuse(t *testing.T) {
	now := time.Now()
	validUntil := now.Add(5 * time.Second)
	session := models.AuthSession{
		PreviousJTIHash:    "previous",
		PreviousValidUntil: &validUntil,
		ExpiresAt:          now.Add(time.Hour),
	}

	if got := classifyRefreshRotation(session, "previous", now); got != models.RefreshRotationPreviousConflict {
		t.Fatalf("grace state = %q", got)
	}
	if got := classifyRefreshRotation(session, "unknown", now); got != models.RefreshRotationReuse {
		t.Fatalf("unknown state = %q", got)
	}
	if got := classifyRefreshRotation(session, "previous", validUntil.Add(time.Second)); got != models.RefreshRotationReuse {
		t.Fatalf("expired grace state = %q", got)
	}
}

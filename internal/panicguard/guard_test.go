package panicguard

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestPanicBecomesErrorWithoutLeakingValue(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	err := Run("test", func() error { panic("private-secret") })
	if !errors.Is(err, ErrPanic) || strings.Contains(err.Error(), "private-secret") || strings.Contains(logs.String(), "private-secret") {
		t.Fatalf("panic error or log leaked value: %v", err)
	}
	if !strings.Contains(logs.String(), "stack") {
		t.Fatal("missing diagnostics")
	}
	sentinel := errors.New("ordinary error")
	if err := Run("test", func() error { return sentinel }); err != sentinel {
		t.Fatalf("ordinary error lost: %v", err)
	}
}
func TestEssentialWorkerPanicStopsService(t *testing.T) {
	stopped := 0
	Essential("test", func() { panic("private") }, func() { stopped++ })
	if stopped != 1 {
		t.Fatalf("stop calls=%d", stopped)
	}
	Essential("normal", func() {}, func() { stopped++ })
	if stopped != 1 {
		t.Fatal("normal return stopped service")
	}
}

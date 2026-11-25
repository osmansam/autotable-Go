package main

import (
	"sync/atomic"
	"testing"
)

func TestWorkerPanicCannotExitSuccessfullyBeforeShutdownCompletes(t *testing.T) {
	var failed atomic.Bool
	shutdownStarted := make(chan struct{})
	finishShutdown := make(chan struct{})
	done := make(chan struct{})
	go func() { defer close(done); failed.Store(true); close(shutdownStarted); <-finishShutdown }()
	<-shutdownStarted
	// The HTTP listener returns while the worker still waits for graceful shutdown.
	status := serverExitCode(func() error { return nil }, &failed)
	close(finishShutdown)
	<-done
	if status != 1 {
		t.Fatalf("worker failure exit status=%d, want nonzero", status)
	}
	var normal atomic.Bool
	if status := serverExitCode(func() error { return nil }, &normal); status != 0 {
		t.Fatalf("normal exit=%d", status)
	}
}

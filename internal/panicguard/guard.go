// Package panicguard contains recovery boundaries for independent units of work.
package panicguard

import (
	"errors"
	"log/slog"
	"runtime/debug"
)

var ErrPanic = errors.New("operation panicked")

// Run converts a panic into an error at the caller's task boundary. Panic values
// can contain credentials or customer data, so only the stack is logged.
func Run(operation string, task func() error) (err error) {
	defer func() {
		if recover() != nil {
			slog.Error("panic recovered", "operation", operation, "stack", string(debug.Stack()))
			err = ErrPanic
		}
	}()
	return task()
}

// Essential stops the service if a long-lived worker panics. Silently losing a
// security/session worker would leave a healthy-looking but degraded process.
func Essential(operation string, worker func(), stop func()) {
	if err := Run(operation, func() error { worker(); return nil }); err != nil {
		stop()
	}
}

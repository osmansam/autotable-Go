package main

import "testing"

func TestCollaborationCLIRequiresExplicitTarget(t *testing.T) {
	for _, args := range [][]string{{}, {"preflight"}, {"apply", "--report-hash", "reviewed"}} {
		if err := run(args); err == nil {
			t.Fatal("CLI inherited an implicit application database")
		}
	}
}

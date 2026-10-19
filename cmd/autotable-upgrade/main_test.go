package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestUpgradeCLIRequiresExplicitTargetWithoutLeakingCredentials(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run(nil, strings.NewReader(""), &out, &errOut, func(string) string { return "" })
	if code == 0 || out.Len() != 0 {
		t.Fatal("audit accepted an implicit database")
	}
	secret := "mongodb://operator:do-not-print-this@localhost/test"
	for _, args := range [][]string{{"--action", "add-password-field", "--database", "test"}, {"--action", "set-password", "--database", "test", "--tenant", "111111111111111111111111", "--project", "222222222222222222222222", "--schema", "auth", "--user", "333333333333333333333333", "--password-stdin"}} {
		out.Reset()
		errOut.Reset()
		code := run(args, strings.NewReader("short"), &out, &errOut, func(string) string { return secret })
		if code == 0 {
			t.Fatal("unscoped/weak enrollment accepted")
		}
		if strings.Contains(out.String()+errOut.String(), secret) || strings.Contains(out.String()+errOut.String(), "short") {
			t.Fatal("CLI exposed credentials/password")
		}
	}
}

func TestUpgradeCLIHelpSucceedsWithoutDatabase(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--help"}, strings.NewReader(""), &out, &errOut, func(string) string { return "" }); code != 0 {
		t.Fatalf("help exit=%d", code)
	}
}

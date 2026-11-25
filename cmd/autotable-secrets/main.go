// Generates new keys locally; it never edits environment files or rotates keys.
package main

import (
	"fmt"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/utils"
	"io"
	"os"
)

func writeSecrets(out io.Writer) error {
	project, err := configs.GenerateJWTSecret()
	if err != nil {
		return err
	}
	tenant, err := configs.GenerateJWTSecret()
	if err != nil {
		return err
	}
	if project == tenant {
		return fmt.Errorf("generated keys must differ")
	}
	_, err = fmt.Fprintf(out, "JWT_SECRET=%s\nTENANT_JWT_SECRET=%s\n", project, tenant)
	return err
}
func main() {
	if err := runSecretCommand(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "Secret operation failed:", err)
		os.Exit(1)
	}
}

// Check-only mode reads configuration and never connects databases, generates
// keys, writes environment files, rotates sessions or prints credential values.
func runSecretCommand(args []string, out io.Writer) error {
	if len(args) == 0 {
		return writeSecrets(out)
	}
	if len(args) != 1 || args[0] != "--check" {
		return fmt.Errorf("usage: autotable-secrets [--check]")
	}
	if err := configs.ValidateJWTSecrets(); err != nil {
		return err
	}
	if _, err := configs.DiagnosticsToken(); err != nil {
		return err
	}
	if key := os.Getenv("EXTERNAL_API_CREDENTIAL_KEY"); key != "" {
		if err := utils.ValidateExternalAPIEncryptionKey(key); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(out, "JWT, diagnostics and encryption key configuration valid; no keys generated or changed.")
	return err
}

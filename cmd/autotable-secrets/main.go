// Generates new keys locally; it never edits environment files or rotates keys.
package main

import (
	"fmt"
	"github.com/osmansam/autotableGo/configs"
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
	if err := writeSecrets(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "Secret generation failed")
		os.Exit(1)
	}
}

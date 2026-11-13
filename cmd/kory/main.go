// Command kory publie et intègre des kory depuis le terminal (TRD §18).
package main

import (
	"os"

	"koryhub.dev/kory/internal/commands"
)

func main() {
	os.Exit(commands.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

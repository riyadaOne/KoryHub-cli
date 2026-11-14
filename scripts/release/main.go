// Commande de construction de kory (K05, décision 2b) : go run ./scripts/release 0.7.0
// Écrit dist/<version>/kory_<os>_<arch>[.exe] pour les 6 cibles et dist/<version>/checksums.txt.
// Ces fichiers sont ensuite déposés sur R2 (07-exploitation/publication-cli.md), ou copiés dans
// KoryHub-web/app/public/cli/<version>/ pour un essai local.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"koryhub.dev/kory/internal/release"
)

func main() {
	if len(os.Args) != 2 || !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(os.Args[1]) {
		fmt.Fprintln(os.Stderr, "usage : go run ./scripts/release x.y.z")
		os.Exit(2)
	}
	version := os.Args[1]
	dir := filepath.Join("dist", version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(err)
	}

	var names []string
	for _, target := range release.Targets {
		name := release.BinaryName(target)
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags", release.LDFlags(version), "-o", filepath.Join(dir, name), "./cmd/kory")
		cmd.Env = append(os.Environ(), "GOOS="+target.OS, "GOARCH="+target.Arch, "CGO_ENABLED=0")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			fail(fmt.Errorf("%s/%s : %w", target.OS, target.Arch, err))
		}
		fmt.Println("✓", name)
		names = append(names, name)
	}

	if err := release.WriteChecksums(dir, names); err != nil {
		fail(err)
	}
	fmt.Println("✓", filepath.Join(dir, "checksums.txt"))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "✗", err)
	os.Exit(1)
}

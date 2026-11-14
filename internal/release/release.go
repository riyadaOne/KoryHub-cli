// Package release construit les exécutables de kory pour la distribution (K05, TRD §18.1,
// décision 2b) : 3 systèmes × 2 architectures, version injectée, fichier checksums.txt (SHA-256).
package release

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Target est un couple système et architecture.
type Target struct {
	OS, Arch string
}

// Targets : Windows, macOS et Linux, en amd64 et arm64 (NF-12).
var Targets = []Target{
	{"windows", "amd64"}, {"windows", "arm64"},
	{"darwin", "amd64"}, {"darwin", "arm64"},
	{"linux", "amd64"}, {"linux", "arm64"},
}

// BinaryName : kory_<os>_<arch>, avec .exe sous Windows ; nom attendu par install.ps1 et install.sh.
func BinaryName(t Target) string {
	name := "kory_" + t.OS + "_" + t.Arch
	if t.OS == "windows" {
		name += ".exe"
	}

	return name
}

// LDFlags : exécutable allégé et version injectée (T-11).
func LDFlags(version string) string {
	return "-s -w -X koryhub.dev/kory/internal/version.Version=" + version
}

// WriteChecksums écrit checksums.txt au format de sha256sum (« empreinte  nom »), trié par nom.
func WriteChecksums(dir string, names []string) error {
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)

	var lines []string
	for _, name := range sorted {
		sum, err := fileSHA256(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		lines = append(lines, sum+"  "+name)
	}

	return os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("%s : %w", path, err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

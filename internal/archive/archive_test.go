package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"koryhub.dev/kory/internal/project"
)

func setup(t *testing.T, files map[string]string) (string, *project.Kory) {
	t.Helper()
	root := t.TempDir()
	var list []string
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(content), 0o644)
		list = append(list, rel)
	}
	readme := filepath.Join(root, ".kory", "paiement", "README.md")
	os.MkdirAll(filepath.Dir(readme), 0o755)
	os.WriteFile(readme, []byte("# paiement\n"), 0o644)

	return root, &project.Kory{
		Manifest:     project.Manifest{Name: "paiement", Version: "1.0.0", License: "MIT", Files: list},
		ManifestPath: filepath.Join(root, ".kory", "paiement", "kory.json"),
		ReadmePath:   readme,
	}
}

func read(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	out := map[string][]byte{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(tr)
		out[h.Name] = b
	}

	return out
}

func TestArchiveKeepsExactBytesRelativeToTheCommonFolder(t *testing.T) {
	crlf := "<?php\r\nfinal class PaymentService {}\r\n"
	root, k := setup(t, map[string]string{
		"src/Payments/PaymentService.php":    crlf,
		"src/Payments/Contracts/Gateway.php": "<?php",
		"src/Payments/docs/flux.png":         "\x89PNG\r\n\x1a\n\x00\x00",
	})

	result, err := Build(root, k, "@moussa/paiement")
	if err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(result.Data)
	if result.SHA256 != hex.EncodeToString(sum[:]) {
		t.Error("empreinte de l'archive")
	}

	files := read(t, result.Data)
	if string(files["PaymentService.php"]) != crlf {
		t.Errorf("octets modifiés : %q", files["PaymentService.php"])
	}
	for name := range files {
		if strings.Contains(name, "src/Payments") || strings.Contains(name, root) {
			t.Errorf("chemin local dans l'archive : %s", name)
		}
	}

	var manifest map[string]any
	json.Unmarshal(files["kory.json"], &manifest)
	if manifest["name"] != "@moussa/paiement" {
		t.Errorf("nom : %v", manifest["name"])
	}
	if raw := string(files["kory.json"]); strings.Contains(raw, "src/Payments") || strings.Contains(raw, filepath.ToSlash(root)) {
		t.Errorf("manifeste non normalisé : %s", raw)
	}
	if _, ok := files["README.md"]; !ok {
		t.Error("README absent")
	}
}

func TestArchiveRefusesBinarySvgConflictAndMissingFiles(t *testing.T) {
	cases := map[string]map[string]string{
		"binary":   {"src/outil.exe": "MZ\x00\x00"},
		"svg":      {"docs/schema.svg": "<svg></svg>"},
		"conflict": {"README.md": "# projet"},
	}
	for code, files := range cases {
		root, k := setup(t, files)
		_, err := Build(root, k, "@moussa/paiement")
		var problem *Problem
		if !errors.As(err, &problem) || problem.Code != code {
			t.Errorf("%s : %v", code, err)
		}
	}

	root, k := setup(t, map[string]string{"a.php": "<?php"})
	k.Manifest.Files = append(k.Manifest.Files, "absent.php", "../hors.php")
	_, err := Build(root, k, "@moussa/paiement")
	var problem *Problem
	if !errors.As(err, &problem) || (problem.Code != "missing" && problem.Code != "outside") {
		t.Errorf("fichier absent : %v", err)
	}
}

func TestArchiveRefusesSymbolicLinks(t *testing.T) {
	root, k := setup(t, map[string]string{"a.php": "<?php"})
	if err := os.Symlink(filepath.Join(root, "a.php"), filepath.Join(root, "lien.php")); err != nil {
		t.Skip("liens symboliques non disponibles sur ce poste")
	}
	k.Manifest.Files = append(k.Manifest.Files, "lien.php")

	_, err := Build(root, k, "@moussa/paiement")
	var problem *Problem
	if !errors.As(err, &problem) || problem.Code != "symlink" {
		t.Errorf("lien : %v", err)
	}
}

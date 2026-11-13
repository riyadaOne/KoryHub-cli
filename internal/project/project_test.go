package project

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInitNamedKoryInDotKoryWithIgnoredFilesLeftOut(t *testing.T) {
	root := t.TempDir()
	write(t, root, "src/Payments/PaymentService.php", "<?php")
	write(t, root, "src/Payments/Contracts/Gateway.php", "<?php")
	write(t, root, "src/Payments/.env", "SECRET=1")
	write(t, root, "src/Payments/vendor/lib.php", "<?php")
	write(t, root, "src/Other.php", "<?php")

	k, err := Init(root, InitOptions{Name: "paiement", Add: []string{"src/Payments"}, License: "MIT", Readme: "# paiement\n"})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"src/Payments/Contracts/Gateway.php", "src/Payments/PaymentService.php"}
	if !slices.Equal(k.Manifest.Files, want) {
		t.Errorf("fichiers %v ; attendu %v", k.Manifest.Files, want)
	}
	if k.Manifest.Language != "php" || k.Manifest.Version != "1.0.0" || k.Manifest.License != "MIT" {
		t.Errorf("manifeste %+v", k.Manifest)
	}
	if _, err := os.Stat(filepath.Join(root, ".kory", "paiement", "README.md")); err != nil {
		t.Error("README absent")
	}
	if _, err := os.Stat(filepath.Join(root, IgnoreFile)); err != nil {
		t.Error(".koryignore absent")
	}

	if _, err := Init(root, InitOptions{Name: "paiement"}); err != ErrExists {
		t.Errorf("deuxième init : %v", err)
	}
	if _, err := Init(root, InitOptions{Name: "Pas Bon"}); err != ErrInvalidName {
		t.Errorf("nom invalide : %v", err)
	}
}

func TestDiscoverFindsRootAndDotKoryManifests(t *testing.T) {
	root := t.TempDir()
	write(t, root, "kory.json", `{"name":"racine","version":"1.0.0","files":[]}`)
	write(t, root, ".kory/sms/kory.json", `{"name":"sms","version":"0.3.1","files":["a.php"]}`)
	write(t, root, ".kory/vide/notes.txt", "pas un kory")

	found, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 || found[0].Manifest.Name != "racine" || found[1].Manifest.Name != "sms" {
		t.Fatalf("trouvés : %+v", found)
	}

	if _, ok := Find(found, ""); ok {
		t.Error("deux kory : un nom est nécessaire")
	}
	if k, ok := Find(found, "sms"); !ok || k.Manifest.Version != "0.3.1" {
		t.Error("sms introuvable")
	}
}

func TestIgnorePatterns(t *testing.T) {
	root := t.TempDir()
	write(t, root, IgnoreFile, "# commentaire\nbuild/\ndocs/brouillon.md\n")
	ig, err := LoadIgnore(root)
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]bool{
		"src/.env.local": true, "keys/server.pem": true, "node_modules/x/index.js": true, "build/app.js": true,
		"docs/brouillon.md": true, "docs/guide.md": false, "src/Service.php": false, ".kory/paiement/kory.json": true,
	}
	for rel, want := range cases {
		if got := ig.Match(rel); got != want {
			t.Errorf("Match(%q) = %v ; attendu %v", rel, got, want)
		}
	}
}

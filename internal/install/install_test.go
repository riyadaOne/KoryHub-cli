package install

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"koryhub.dev/kory/internal/lock"
)

func tarball(t *testing.T, entries map[string]string, extra func(tw *tar.Writer)) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range entries {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg})
		tw.Write([]byte(content))
	}
	if extra != nil {
		extra(tw)
	}
	tw.Close()
	gz.Close()

	return buf.Bytes()
}

func TestReadKeepsTheFilesButNotKoryJSON(t *testing.T) {
	a, err := Read(tarball(t, map[string]string{
		"kory.json":              `{"name":"@moussa/paiement","version":"1.2.0"}`,
		"README.md":              "# paiement\n",
		"src/PaymentService.php": "<?php\r\n",
	}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "@moussa/paiement" || a.Version != "1.2.0" || len(a.Files) != 2 || string(a.Files["src/PaymentService.php"]) != "<?php\r\n" {
		t.Errorf("archive : %+v", a)
	}
}

func TestReadRefusesPathsOutsideTheKoryAndLinks(t *testing.T) {
	cases := map[string]func(tw *tar.Writer){
		"..": func(tw *tar.Writer) {
			tw.WriteHeader(&tar.Header{Name: "../../etc/passwd", Size: 0, Typeflag: tar.TypeReg})
		},
		"absolu": func(tw *tar.Writer) { tw.WriteHeader(&tar.Header{Name: "/tmp/x", Size: 0, Typeflag: tar.TypeReg}) },
		"windows": func(tw *tar.Writer) {
			tw.WriteHeader(&tar.Header{Name: "C:/Windows/x", Size: 0, Typeflag: tar.TypeReg})
		},
		"lien": func(tw *tar.Writer) {
			tw.WriteHeader(&tar.Header{Name: "src/x", Linkname: "/etc/passwd", Typeflag: tar.TypeSymlink})
		},
	}
	for name, extra := range cases {
		_, err := Read(tarball(t, map[string]string{"kory.json": `{"name":"@a/b","version":"1.0.0"}`}, extra))
		if !errors.Is(err, ErrUnsafeArchive) {
			t.Errorf("%s : %v", name, err)
		}
	}
}

func TestPlanThenWriteNeverOverwritesWithoutAChoice(t *testing.T) {
	root := t.TempDir()
	dir := "kory/paiement"
	os.MkdirAll(filepath.Join(root, "kory", "paiement", "src"), 0o755)
	os.WriteFile(filepath.Join(root, "kory", "paiement", "src", "Same.php"), []byte("same"), 0o644)
	os.WriteFile(filepath.Join(root, "kory", "paiement", "src", "Mine.php"), []byte("mine"), 0o644)
	os.WriteFile(filepath.Join(root, "kory", "paiement", "src", "Other.php"), []byte("other"), 0o644)

	files := map[string][]byte{
		"src/New.php":   []byte("new"),
		"src/Same.php":  []byte("same"),
		"src/Mine.php":  []byte("kory"),
		"src/Other.php": []byte("kory"),
	}
	items, err := Plan(root, dir, files, nil)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]State{}
	for _, i := range items {
		states[i.Path] = i.State
	}
	if states["src/New.php"] != New || states["src/Same.php"] != Identical || states["src/Mine.php"] != Conflict {
		t.Fatalf("états : %v", states)
	}

	result, err := Write(root, dir, items, map[string]bool{"src/Other.php": true})
	if err != nil {
		t.Fatal(err)
	}
	read := func(p string) string {
		b, _ := os.ReadFile(filepath.Join(root, "kory", "paiement", filepath.FromSlash(p)))
		return string(b)
	}
	if read("src/New.php") != "new" || read("src/Mine.php") != "mine" || read("src/Other.php") != "kory" {
		t.Errorf("fichiers : %q %q %q", read("src/New.php"), read("src/Mine.php"), read("src/Other.php"))
	}
	if result.Written != 1 || result.Replaced != 1 || result.Kept != 1 || result.Identical != 1 || !result.Files["src/Mine.php"].Kept {
		t.Errorf("bilan : %+v", result)
	}
}

func TestPlanForAnUpdateReplacesUntouchedFilesAndAsksForModifiedOnes(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "k"), 0o755)
	os.WriteFile(filepath.Join(root, "k", "a.php"), []byte("v1"), 0o644)
	os.WriteFile(filepath.Join(root, "k", "b.php"), []byte("v1 modifié"), 0o644)
	previous := &lock.Entry{Files: map[string]lock.File{"a.php": {SHA256: Hash([]byte("v1"))}, "b.php": {SHA256: Hash([]byte("v1"))}}}

	items, _ := Plan(root, "k", map[string][]byte{"a.php": []byte("v2"), "b.php": []byte("v2")}, previous)
	if items[0].State != Untouched || items[1].State != Modified || !items[1].NeedsChoice() || items[0].NeedsChoice() {
		t.Errorf("états : %v %v", items[0].State, items[1].State)
	}
}

func TestTargetStaysInsideTheProject(t *testing.T) {
	root := t.TempDir()
	for _, bad := range [][2]string{{"..", "x"}, {"kory", "../x"}, {"C:/x", "y"}, {"/abs", "y"}, {strings.Repeat("a/", 3) + "..", "x"}} {
		if _, err := Target(root, bad[0], bad[1]); !errors.Is(err, ErrOutsideProject) {
			t.Errorf("%v : %v", bad, err)
		}
	}
	if _, err := Target(root, ".", "src/a.php"); err != nil {
		t.Errorf("racine : %v", err)
	}
}

func TestLongWindowsPathsAreWritten(t *testing.T) {
	root := t.TempDir()
	long := strings.Repeat("dossier-tres-long/", 16) + "Fichier.php"
	items, err := Plan(root, "kory/paiement", map[string][]byte{long: []byte("x")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Write(root, "kory/paiement", items, nil); err != nil {
		t.Fatalf("chemin de %d caractères : %v", len(filepath.Join(root, long)), err)
	}
}

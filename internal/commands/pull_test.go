package commands

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"koryhub.dev/kory/internal/lock"
)

// fakeHub imite l'API de téléchargement : module, version, archive, erreurs.
type fakeHub struct {
	mu        sync.Mutex
	files     map[string]string // fichiers du kory (sans kory.json)
	extra     func(tw *tar.Writer)
	archive   []byte
	sha       string // empreinte annoncée (vide : celle de l'archive)
	status    string // statut de la version
	failWith  string // corps d'erreur JSON du téléchargement
	failCode  int
	cuts      int // coupures réseau avant de réussir
	downloads int
	version   string // dernière version stable servie
	audit     string // corps de la réponse de POST /v1/audit
	audited   []string
	server    *httptest.Server
}

func newFakeHub(t *testing.T, files map[string]string) *fakeHub {
	t.Helper()
	h := &fakeHub{files: files, status: "published", version: "1.2.0", audit: `{"results":[]}`}
	previous := retryPause
	retryPause = func(int) {}
	t.Cleanup(func() { retryPause = previous })

	h.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.build()
		w.Header().Set("Content-Type", "application/json")

		versionPath := "/v1/modules/moussa/paiement/versions/" + h.version
		switch r.URL.Path {
		case "/v1/audit":
			var body struct {
				Modules []struct{ Name, Version string } `json:"modules"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			for _, m := range body.Modules {
				h.audited = append(h.audited, m.Name+"@"+m.Version)
			}
			w.Write([]byte(h.audit))
		case "/v1/modules/moussa/paiement":
			w.Write([]byte(`{"name":"@moussa/paiement","visibility":"public","status":"active","latest_version":"` + h.version + `","description":"Encaissement Mobile Money.","language":"php","certified":false,"download_count":3,"versions":[]}`))
		case versionPath:
			detail := map[string]any{
				"name": "@moussa/paiement", "version": h.version, "status": h.status, "prerelease": false, "license": "MIT",
				"language": "php", "published_at": nil, "sha256": h.announced(), "size": len(h.archive), "deprecation_message": nil,
				"requires": []map[string]any{{"ecosystem": "composer", "name": "php", "version": ">=8.3"}},
				"files":    h.fileList(),
			}
			json.NewEncoder(w).Encode(detail)
		case versionPath + "/download":
			h.downloads++
			if h.failCode != 0 {
				w.WriteHeader(h.failCode)
				w.Write([]byte(h.failWith))
				return
			}
			http.Redirect(w, r, "/storage/archive.tar.gz?signature=x", http.StatusFound)
		case "/storage/archive.tar.gz":
			if h.cuts > 0 {
				h.cuts--
				w.Header().Set("Content-Length", "100000")
				w.WriteHeader(http.StatusOK)
				w.Write(h.archive[:10])
				w.(http.Flusher).Flush()
				panic(http.ErrAbortHandler) // coupe la connexion au milieu
			}
			w.Header().Set("Content-Type", "application/gzip")
			w.Write(h.archive)
		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message":"Not found.","code":"not_found"}`))
		}
	}))
	t.Cleanup(h.server.Close)

	return h
}

func (h *fakeHub) build() {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	all := map[string]string{"kory.json": `{"name":"@moussa/paiement","version":"` + h.version + `"}`}
	for k, v := range h.files {
		all[k] = v
	}
	names := make([]string, 0, len(all))
	for k := range all {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(all[name])), Typeflag: tar.TypeReg})
		tw.Write([]byte(all[name]))
	}
	if h.extra != nil {
		h.extra(tw)
	}
	tw.Close()
	gz.Close()
	h.archive = buf.Bytes()
}

func (h *fakeHub) announced() string {
	if h.sha != "" {
		return h.sha
	}
	sum := sha256.Sum256(h.archive)

	return hex.EncodeToString(sum[:])
}

func (h *fakeHub) fileList() []map[string]any {
	var list []map[string]any
	for path, content := range h.files {
		sum := sha256.Sum256([]byte(content))
		list = append(list, map[string]any{"path": path, "size": len(content), "sha256": hex.EncodeToString(sum[:])})
	}

	return list
}

func emptyProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)

	return root
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))

	return string(data)
}

var koryFiles = map[string]string{
	"README.md":              "# paiement\n",
	"src/PaymentService.php": "<?php\r\nfinal class PaymentService {}\r\n",
}

func TestPullInstallsInKoryFolderAndWritesTheLockLast(t *testing.T) {
	hub := newFakeHub(t, koryFiles)
	s := newSession(t, hub.server.URL)
	root := emptyProject(t)

	code, out, errOut := s.run("", "pull", "@moussa/paiement", "--yes")
	if code != ExitOK {
		t.Fatalf("code %d, %q, %q", code, out, errOut)
	}
	for _, want := range []string{"@moussa/paiement 1.2.0 · MIT · php · files: 1", "Kory not certified", "composer · php >=8.3", "Checksum verified", "Files written in kory", "kory.lock updated"} {
		if !strings.Contains(out, want) {
			t.Errorf("sortie sans %q : %q", want, out)
		}
	}
	if readFile(t, root, "kory/paiement/src/PaymentService.php") != koryFiles["src/PaymentService.php"] {
		t.Error("fichier non écrit à l'octet près")
	}
	if _, err := os.Stat(filepath.Join(root, "kory", "paiement", "kory.json")); err == nil {
		t.Error("kory.json ne doit pas être écrit")
	}

	l, _ := lock.Load(root)
	entry := l.Modules["@moussa/paiement"]
	if entry.Version != "1.2.0" || entry.Directory != "kory/paiement" || entry.SHA256 != hub.announced() || len(entry.Files) != 2 {
		t.Errorf("kory.lock : %+v", entry)
	}
	if entries, _ := os.ReadDir(filepath.Join(root, ".kory", "tmp")); len(entries) != 0 {
		t.Errorf(".kory/tmp non vidé : %v", entries)
	}

	code, out, _ = s.run("", "pull", "@moussa/paiement", "--yes")
	if code != ExitOK || !strings.Contains(out, "already installed") || hub.downloads != 1 {
		t.Errorf("deuxième pull : code %d, %q, %d téléchargements", code, out, hub.downloads)
	}
}

func TestPullWithYesNeverOverwritesAnExistingFile(t *testing.T) {
	hub := newFakeHub(t, koryFiles)
	s := newSession(t, hub.server.URL)
	root := emptyProject(t)
	os.MkdirAll(filepath.Join(root, "lib", "src"), 0o755)
	os.WriteFile(filepath.Join(root, "lib", "src", "PaymentService.php"), []byte("<?php // le mien\n"), 0o644)

	code, out, errOut := s.run("", "pull", "@moussa/paiement", "--dir", "lib", "--yes")
	if code != ExitOK || !strings.Contains(out, "Existing files kept: 1") {
		t.Fatalf("code %d, %q, %q", code, out, errOut)
	}
	if readFile(t, root, "lib/src/PaymentService.php") != "<?php // le mien\n" {
		t.Error("fichier existant écrasé")
	}
	l, _ := lock.Load(root)
	if !l.Modules["@moussa/paiement"].Files["src/PaymentService.php"].Kept {
		t.Error("fichier gardé non noté dans kory.lock")
	}
}

func TestPullRefusesWithoutWritingAnything(t *testing.T) {
	cases := map[string]struct {
		setup func(h *fakeHub)
		want  string
		code  int
	}{
		"empreinte": {func(h *fakeHub) { h.sha = strings.Repeat("0", 64) }, "Different checksum", ExitError},
		"limite": {func(h *fakeHub) {
			h.failCode, h.failWith = 429, `{"message":"Limit of 60 downloads per hour reached for your address.","code":"download_limit_guest","retry_at":"2026-11-14T15:42:00Z"}`
		}, "kory login", ExitError},
		"retirée": {func(h *fakeHub) {
			h.status = "removed"
			h.failCode, h.failWith = 410, `{"message":"Version 1.2.0 was removed for legal reasons.","code":"version_removed"}`
		}, "removed for legal reasons", ExitError},
		"coupure": {func(h *fakeHub) { h.cuts = 3 }, "Network cut during the download", ExitError},
		"lien": {func(h *fakeHub) {
			h.extra = func(tw *tar.Writer) { tw.WriteHeader(&tar.Header{Name: "../evil", Typeflag: tar.TypeReg}) }
		}, "Archive refused", ExitError},
		"sans terminal": {func(h *fakeHub) {}, "--yes", ExitUsage},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			hub := newFakeHub(t, koryFiles)
			tc.setup(hub)
			s := newSession(t, hub.server.URL)
			root := emptyProject(t)

			args := []string{"pull", "@moussa/paiement", "--yes"}
			if name == "sans terminal" {
				args = args[:2]
			}
			code, out, errOut := s.run("", args...)
			if code != tc.code || !strings.Contains(out+errOut, tc.want) {
				t.Fatalf("code %d, %q, %q", code, out, errOut)
			}
			if _, err := os.Stat(filepath.Join(root, "kory")); err == nil {
				t.Error("des fichiers ont été écrits")
			}
			if _, err := os.Stat(filepath.Join(root, "kory.lock")); err == nil {
				t.Error("kory.lock écrit")
			}
		})
	}
}

func TestPullRetriesAfterANetworkCut(t *testing.T) {
	hub := newFakeHub(t, koryFiles)
	hub.cuts = 2
	s := newSession(t, hub.server.URL)
	emptyProject(t)

	code, out, errOut := s.run("", "pull", "@moussa/paiement", "--yes")
	if code != ExitOK || !strings.Contains(out, "new attempt (2/3)") || !strings.Contains(out, "new attempt (3/3)") {
		t.Fatalf("code %d, %q, %q", code, out, errOut)
	}
}

package commands

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeRegistry imite l'API de publication : compte, module, envoi d'une version.
type fakeRegistry struct {
	mu            sync.Mutex
	termsAccepted bool
	moduleExists  bool
	reject        string // code d'erreur renvoyé à l'envoi
	posts         []map[string]string
	archives      [][]byte
	server        *httptest.Server
}

func newFakeRegistry(t *testing.T) *fakeRegistry {
	t.Helper()
	f := &fakeRegistry{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/user":
			terms := "false"
			if f.termsAccepted {
				terms = "true"
			}
			w.Write([]byte(`{"handle":"moussa","name":"Moussa","token_name":"kory · Windows","token_expires_at":null,"publication_terms_accepted":` + terms + `}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/modules/moussa/paiement":
			if !f.moduleExists {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"message":"Not found.","code":"not_found"}`))
				return
			}
			w.Write([]byte(`{"name":"@moussa/paiement","visibility":"public","status":"active","latest_version":"0.9.0"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/modules/moussa/paiement/versions":
			r.ParseMultipartForm(32 << 20)
			fields := map[string]string{}
			for key, values := range r.MultipartForm.Value {
				fields[key] = values[0]
			}
			file, _, _ := r.FormFile("archive")
			data, _ := io.ReadAll(file)
			f.posts = append(f.posts, fields)
			f.archives = append(f.archives, data)
			if f.reject != "" {
				w.WriteHeader(http.StatusUnprocessableEntity)
				w.Write([]byte(`{"message":"Secret detected: src/config.php line 2 (generic-api-key).","code":"` + f.reject + `"}`))
				return
			}
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"name":"@moussa/paiement","version":"1.0.0","sha256":"` + fields["sha256"] + `","files_count":1,"size":300,"url":"http://koryhub.test/@moussa/paiement"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message":"Not found.","code":"not_found"}`))
		}
	}))
	t.Cleanup(f.server.Close)

	return f
}

// projectWithKory crée un projet avec un kory « paiement » et se place dedans.
func projectWithKory(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "src", "Payments"), 0o755)
	os.WriteFile(filepath.Join(root, "src", "Payments", "PaymentService.php"), []byte("<?php\r\n"), 0o644)
	t.Chdir(root)

	return root
}

func TestInitThenListShowTheKoryOfTheProject(t *testing.T) {
	registry := newFakeRegistry(t)
	s := newSession(t, registry.server.URL)
	projectWithKory(t)

	code, out, errOut := s.run("", "init", "paiement", "--add", "src/Payments")
	if code != ExitOK || !strings.Contains(out, "1") || !strings.Contains(out, "kory push paiement --dry-run") {
		t.Fatalf("init : code %d, %q, %q", code, out, errOut)
	}

	code, out, _ = s.run("", "list")
	if code != ExitOK || !strings.Contains(out, "paiement") || !strings.Contains(out, "—") {
		t.Errorf("list : code %d, %q", code, out)
	}
}

func TestPushDryRunShowsEverythingAndSendsNothing(t *testing.T) {
	registry := newFakeRegistry(t)
	s := newSession(t, registry.server.URL)
	projectWithKory(t)
	s.run("", "init", "paiement", "--add", "src/Payments")

	code, out, errOut := s.run("", "push", "--dry-run")

	if code != ExitOK || !strings.Contains(out, "PaymentService.php") || !strings.Contains(out, "Nothing was sent") {
		t.Fatalf("code %d, %q, %q", code, out, errOut)
	}
	if len(registry.posts) != 0 {
		t.Error("--dry-run ne doit rien envoyer")
	}
}

func TestFirstPushAsksForTermsThenPublishesAsPrivate(t *testing.T) {
	registry := newFakeRegistry(t)
	s := newSession(t, registry.server.URL)
	projectWithKory(t)
	s.run("", "init", "paiement", "--add", "src/Payments")
	registry.termsAccepted = true
	s.run("kory_x", "login", "--with-token")
	registry.termsAccepted = false

	code, _, errOut := s.run("", "push", "paiement")
	if code != ExitUsage || !strings.Contains(errOut, "--yes") {
		t.Fatalf("sans terminal ni --yes : code %d, %q", code, errOut)
	}

	code, out, errOut := s.run("", "push", "paiement", "--yes")
	if code != ExitOK || !strings.Contains(out, "Published @moussa/paiement 1.0.0") || !strings.Contains(out, "First publication") {
		t.Fatalf("code %d, %q, %q", code, out, errOut)
	}

	sent := registry.posts[0]
	if sent["rights_certified"] != "1" || sent["notice_accepted"] != "1" || sent["visibility"] != "private" || len(sent["sha256"]) != 64 {
		t.Errorf("champs envoyés : %v", sent)
	}
	if strings.Contains(string(registry.archives[0]), "src/Payments") {
		t.Error("chemin local envoyé")
	}
}

func TestPushShowsTheServerRefusal(t *testing.T) {
	registry := newFakeRegistry(t)
	registry.termsAccepted, registry.moduleExists, registry.reject = true, true, "secret_detected"
	s := newSession(t, registry.server.URL)
	projectWithKory(t)
	s.run("", "init", "paiement", "--add", "src/Payments")
	s.run("kory_x", "login", "--with-token")

	code, out, errOut := s.run("", "push", "--yes")

	if code != ExitError || !strings.Contains(errOut, "Secret detected: src/config.php line 2") {
		t.Errorf("code %d, %q", code, errOut)
	}
	if !strings.Contains(out, "latest published version: 0.9.0") {
		t.Errorf("sortie : %q", out)
	}
	if _, ok := registry.posts[0]["visibility"]; ok {
		t.Error("module existant : pas de visibilité envoyée")
	}
}

func TestPushWithoutLoginOrKoryIsExplained(t *testing.T) {
	registry := newFakeRegistry(t)
	s := newSession(t, registry.server.URL)
	projectWithKory(t)

	code, _, errOut := s.run("", "push")
	if code != ExitUsage || !strings.Contains(errOut, "kory init") {
		t.Errorf("sans kory : code %d, %q", code, errOut)
	}

	s.run("", "init", "paiement", "--add", "src/Payments")
	code, _, errOut = s.run("", "push", "--yes")
	if code != ExitError || !strings.Contains(errOut, "kory login") {
		t.Errorf("sans connexion : code %d, %q", code, errOut)
	}
}

func TestPushWarnsAboutInternalInformationLineByLine(t *testing.T) {
	registry := newFakeRegistry(t)
	s := newSession(t, registry.server.URL)
	root := projectWithKory(t)
	os.WriteFile(filepath.Join(root, "src", "Payments", "Gateway.php"), []byte("<?php\n$cb = 'https://api-prod.client.td/callback';\n"), 0o644)
	s.run("", "init", "paiement", "--add", "src/Payments")

	_, out, _ := s.run("", "push", "--dry-run")

	if !strings.Contains(out, "Gateway.php:2") || !strings.Contains(out, "internal web address") || !strings.Contains(out, "https://api-prod.client.td/callback") {
		t.Errorf("sortie : %q", out)
	}
}

func TestASecretBlocksPushAndDryRunWithoutSendingAnything(t *testing.T) {
	registry := newFakeRegistry(t)
	registry.termsAccepted = true
	s := newSession(t, registry.server.URL)
	root := projectWithKory(t)
	token := "kory_" + strings.Repeat("A", 40) + "0123abcd"
	os.WriteFile(filepath.Join(root, "src", "Payments", "ci.php"), []byte("<?php\n\n$t = '"+token+"';\n"), 0o644)
	s.run("", "init", "paiement", "--add", "src/Payments")
	s.run("kory_x", "login", "--with-token")

	for _, args := range [][]string{{"push", "--dry-run"}, {"push", "--yes"}} {
		code, out, errOut := s.run("", args...)
		if code != ExitError || !strings.Contains(errOut, "Secret detected: ci.php line 3 (koryhub-cli-token)") || strings.Contains(out+errOut, token) {
			t.Errorf("%v : code %d, %q, %q", args, code, out, errOut)
		}
	}
	if len(registry.posts) != 0 {
		t.Error("rien ne doit être envoyé")
	}
}

func TestGraphWritesTheArchitectureAndPushPublishesItRelativeToTheKory(t *testing.T) {
	registry := newFakeRegistry(t)
	s := newSession(t, registry.server.URL)
	root := projectWithKory(t)
	os.WriteFile(filepath.Join(root, "src", "Payments", "PaymentService.php"), []byte("<?php\nfinal class PaymentService\n{\n    public function __construct(private PaymentGateway $g) {}\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "src", "Payments", "PaymentGateway.php"), []byte("<?php\ninterface PaymentGateway\n{\n}\n"), 0o644)
	s.run("", "init", "paiement", "--add", "src/Payments")

	code, _, errOut := s.run("", "graph", "paiement")
	if code != ExitUsage || !strings.Contains(errOut, "--yes") {
		t.Fatalf("sans terminal ni --yes : code %d, %q", code, errOut)
	}

	code, out, _ := s.run("", "graph", "paiement", "--yes")
	if code != ExitOK || !strings.Contains(out, "2 components, 1 links") || !strings.Contains(out, "PaymentService → PaymentGateway  (uses)") {
		t.Fatalf("graph : code %d, %q", code, out)
	}

	manifest, _ := os.ReadFile(filepath.Join(root, ".kory", "paiement", "kory.json"))
	if !strings.Contains(string(manifest), `"file": "src/Payments/PaymentService.php"`) {
		t.Errorf("manifeste local : %s", manifest)
	}

	registry.termsAccepted = true
	s.run("kory_x", "login", "--with-token")
	s.run("", "push", "--yes")
	published := koryJSON(t, registry.archives[0])
	if !strings.Contains(published, `"file": "PaymentService.php"`) || strings.Contains(published, "src/Payments") {
		t.Errorf("kory.json publié : %s", published)
	}
}

// koryJSON lit kory.json dans une archive .tar.gz envoyée.
func koryJSON(t *testing.T, data []byte) string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			t.Fatal("kory.json absent")
		}
		if h.Name == "kory.json" {
			b, _ := io.ReadAll(tr)
			return string(b)
		}
	}
}

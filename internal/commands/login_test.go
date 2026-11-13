package commands

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"koryhub.dev/kory/internal/browser"
)

// fakeAPI imite l'API de connexion : polls réponses successives à /v1/auth/device/token.
type fakeAPI struct {
	mu      sync.Mutex
	polls   []string
	revoked []string
	tokens  map[string]bool
	server  *httptest.Server
}

func newFakeAPI(t *testing.T, polls ...string) *fakeAPI {
	t.Helper()
	f := &fakeAPI{polls: polls, tokens: map[string]bool{"kory_ancien": true}}

	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/device":
			w.Write([]byte(`{"device_code":"dc","user_code":"WDJB-MJHT","verification_uri":"http://koryhub.test/login/device","verification_uri_complete":"http://koryhub.test/login/device?code=WDJB-MJHT","expires_in":900,"interval":5}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/device/token":
			next := f.polls[0]
			f.polls = f.polls[1:]
			if next == "ok" {
				f.tokens["kory_nouveau"] = true
				w.Write([]byte(`{"token":"kory_nouveau","token_name":"kory · windows","expires_at":"2099-01-01T00:00:00Z","handle":"moussa"}`))

				return
			}
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"message":"` + next + `","code":"` + next + `"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/user":
			if !f.tokens[token] {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"message":"Your token is no longer valid.","code":"token_invalid"}`))

				return
			}
			w.Write([]byte(`{"handle":"moussa","name":"Moussa","token_name":"CI GitHub","token_expires_at":null}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/auth/token":
			f.revoked = append(f.revoked, token)
			delete(f.tokens, token)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.server.Close)

	return f
}

// session garde le même dossier de configuration entre plusieurs commandes.
type session struct {
	t      *testing.T
	dir    string
	apiURL string
}

func newSession(t *testing.T, apiURL string) *session {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("KORY_CONFIG_DIR", dir)
	t.Setenv("KORY_API_URL", apiURL)
	t.Setenv("LANG", "en_US.UTF-8")

	previousWait, previousOpen := wait, browser.Open
	wait = func(time.Duration) {}
	browser.Open = func(string) error { return nil }
	t.Cleanup(func() { wait, browser.Open = previousWait, previousOpen })

	return &session{t: t, dir: dir, apiURL: apiURL}
}

func (s *session) run(stdin string, args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := Run(args, strings.NewReader(stdin), &out, &errOut)

	return code, out.String(), errOut.String()
}

func (s *session) credentialFile() string {
	return filepath.Join(s.dir, "credentials")
}

func TestLoginWithADeviceCodeWaitsThenSavesTheToken(t *testing.T) {
	api := newFakeAPI(t, "authorization_pending", "slow_down", "ok")
	s := newSession(t, api.server.URL)

	opened := ""
	browser.Open = func(url string) error { opened = url; return nil }

	code, out, errOut := s.run("", "login")

	if code != ExitOK {
		t.Fatalf("code %d, %q", code, errOut)
	}
	for _, want := range []string{"WDJB-MJHT", "http://koryhub.test/login/device", "Logged in as @moussa", "kory · windows", "2099-01-01", s.credentialFile()} {
		if !strings.Contains(out, want) {
			t.Errorf("sortie sans %q : %q", want, out)
		}
	}
	if opened != "http://koryhub.test/login/device?code=WDJB-MJHT" {
		t.Errorf("navigateur ouvert sur %q", opened)
	}
	if data, _ := os.ReadFile(s.credentialFile()); !strings.Contains(string(data), "kory_nouveau") {
		t.Errorf("fichier : %s", data)
	}
}

func TestLoginShowsTheAddressWhenNoBrowserCanBeOpened(t *testing.T) {
	api := newFakeAPI(t, "ok")
	s := newSession(t, api.server.URL)
	browser.Open = func(string) error { return errors.New("pas de navigateur") }

	_, out, _ := s.run("", "login")

	if !strings.Contains(out, "on this computer or another one") {
		t.Errorf("sortie : %q", out)
	}
}

func TestLoginStopsWhenTheCodeExpiresOrIsRefused(t *testing.T) {
	for code, want := range map[string]string{"expired_token": "The code has expired.", "access_denied": "Login was refused on the site."} {
		api := newFakeAPI(t, "authorization_pending", code)
		s := newSession(t, api.server.URL)

		exit, _, errOut := s.run("", "login")

		if exit != ExitError || !strings.Contains(errOut, want) || !strings.Contains(errOut, "kory login") {
			t.Errorf("%s : code %d, %q", code, exit, errOut)
		}
		if _, err := os.Stat(s.credentialFile()); err == nil {
			t.Errorf("%s : aucun jeton ne doit être enregistré", code)
		}
	}
}

func TestLoginWithTokenReadsStandardInputAndChecksTheToken(t *testing.T) {
	api := newFakeAPI(t)
	api.tokens["kory_ci"] = true
	s := newSession(t, api.server.URL)

	code, out, errOut := s.run("kory_ci\n", "login", "--with-token")

	if code != ExitOK || !strings.Contains(out, "@moussa") || !strings.Contains(out, "never") {
		t.Fatalf("code %d, %q, %q", code, out, errOut)
	}

	code, _, errOut = s.run("kory_faux\n", "login", "--with-token")
	if code != ExitError || !strings.Contains(errOut, "no longer valid") || !strings.Contains(errOut, "kory login") {
		t.Errorf("jeton invalide : code %d, %q", code, errOut)
	}

	code, _, errOut = s.run("", "login", "--with-token")
	if code != ExitUsage || !strings.Contains(errOut, "No token received") {
		t.Errorf("sans jeton : code %d, %q", code, errOut)
	}
}

func TestLoginAgainAsksBeforeReplacingAndRevokesTheOldToken(t *testing.T) {
	api := newFakeAPI(t, "ok")
	s := newSession(t, api.server.URL)
	s.run("kory_ancien", "login", "--with-token")

	code, _, errOut := s.run("", "login")
	if code != ExitUsage || !strings.Contains(errOut, "--yes") {
		t.Fatalf("sans terminal : code %d, %q", code, errOut)
	}

	if code, _, errOut := s.run("", "login", "--yes"); code != ExitOK {
		t.Fatalf("--yes : code %d, %q", code, errOut)
	}
	if len(api.revoked) != 1 || api.revoked[0] != "kory_ancien" {
		t.Errorf("révoqués : %v", api.revoked)
	}
}

func TestWhoamiShowsTheAccountOrSaysToLogIn(t *testing.T) {
	api := newFakeAPI(t)
	api.tokens["kory_ci"] = true
	s := newSession(t, api.server.URL)

	code, _, errOut := s.run("", "whoami")
	if code != ExitError || !strings.Contains(errOut, "not connected") || !strings.Contains(errOut, "kory login") {
		t.Errorf("non relié : code %d, %q", code, errOut)
	}

	s.run("kory_ci", "login", "--with-token")
	code, out, _ := s.run("", "whoami")
	if code != ExitOK || !strings.HasPrefix(out, "@moussa") || !strings.Contains(out, api.server.URL) || !strings.Contains(out, "CI GitHub") {
		t.Errorf("relié : code %d, %q", code, out)
	}

	delete(api.tokens, "kory_ci")
	code, _, errOut = s.run("", "whoami")
	if code != ExitError || !strings.Contains(errOut, "no longer valid") || !strings.Contains(errOut, "kory login") {
		t.Errorf("révoqué : code %d, %q", code, errOut)
	}
}

func TestLogoutRevokesThenRemovesTheToken(t *testing.T) {
	api := newFakeAPI(t)
	api.tokens["kory_ci"] = true
	s := newSession(t, api.server.URL)
	s.run("kory_ci", "login", "--with-token")

	code, out, _ := s.run("", "logout")

	if code != ExitOK || !strings.Contains(out, "Token revoked") || len(api.revoked) != 1 {
		t.Errorf("code %d, %q, révoqués %v", code, out, api.revoked)
	}
	if _, err := os.Stat(s.credentialFile()); err == nil {
		t.Error("le fichier du jeton devrait être effacé")
	}

	_, out, _ = s.run("", "logout")
	if !strings.Contains(out, "was not connected") {
		t.Errorf("deuxième logout : %q", out)
	}
}

func TestLogoutOfflineRemovesTheTokenAndSaysWhereToRevokeIt(t *testing.T) {
	api := newFakeAPI(t)
	api.tokens["kory_ci"] = true
	s := newSession(t, api.server.URL)
	s.run("kory_ci", "login", "--with-token")
	api.server.Close()

	code, out, _ := s.run("", "logout")

	if code != ExitOK || !strings.Contains(out, "still valid") || !strings.Contains(out, "/settings/tokens") {
		t.Errorf("code %d, %q", code, out)
	}
	if _, err := os.Stat(s.credentialFile()); err == nil {
		t.Error("le fichier du jeton devrait être effacé")
	}
}

func TestCredentialFileSettingMovesTheTokenAndMustBeAbsolute(t *testing.T) {
	api := newFakeAPI(t)
	api.tokens["kory_ci"] = true
	s := newSession(t, api.server.URL)
	custom := filepath.Join(t.TempDir(), "jeton-kory")

	if code, _, errOut := s.run("", "config", "set", "credential-file", "relatif/jeton"); code != ExitUsage || !strings.Contains(errOut, "Invalid path") {
		t.Errorf("chemin relatif : code %d, %q", code, errOut)
	}

	s.run("", "config", "set", "credential-file", custom)
	s.run("kory_ci", "login", "--with-token")

	if _, err := os.Stat(custom); err != nil {
		t.Errorf("jeton absent de %s : %v", custom, err)
	}
}

func TestSiteURLIsDeducedFromTheAPIAddress(t *testing.T) {
	cases := map[string]string{
		"https://api.koryhub.dev":       "https://koryhub.dev",
		"http://api.koryhub.test:8000/": "http://koryhub.test:8000",
	}
	for in, want := range cases {
		if got := siteURL(in); got != want {
			t.Errorf("siteURL(%q) = %q ; attendu %q", in, got, want)
		}
	}
}

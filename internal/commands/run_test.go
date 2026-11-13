package commands

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"koryhub.dev/kory/internal/version"
)

// runKory lance kory dans un dossier de configuration vide, contre l'API donnée.
func runKory(t *testing.T, apiURL string, args ...string) (int, string, string) {
	t.Helper()
	t.Setenv("KORY_CONFIG_DIR", t.TempDir())
	t.Setenv("KORY_API_URL", apiURL)
	t.Setenv("LANG", "en_US.UTF-8")

	var out, errOut bytes.Buffer
	code := Run(args, strings.NewReader(""), &out, &errOut)

	return code, out.String(), errOut.String()
}

func metaServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	return server
}

func TestVersionShowsTheVersionAndANewerRelease(t *testing.T) {
	server := metaServer(t, 200, `{"api_version":"v1","minimum_cli_version":"0.1.0","latest_cli_version":"9.0.0"}`)

	code, out, _ := runKory(t, server.URL, "--version")

	if code != ExitOK {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(out, "kory "+version.Version) || !strings.Contains(out, "9.0.0") {
		t.Errorf("sortie : %q", out)
	}
}

func TestVersionUpToDateSaysNothingMore(t *testing.T) {
	server := metaServer(t, 200, `{"api_version":"v1","minimum_cli_version":"0.1.0","latest_cli_version":"`+version.Version+`"}`)

	_, out, _ := runKory(t, server.URL, "version")

	if strings.TrimSpace(out) != "kory "+version.Version {
		t.Errorf("sortie : %q", out)
	}
}

func TestTooOldCliShowsTheServerMessageAndHowToUpdate(t *testing.T) {
	server := metaServer(t, 426, `{"message":"Update kory: this version is no longer supported.","code":"upgrade_required"}`)

	code, _, errOut := runKory(t, server.URL, "version")

	if code != ExitError || !strings.Contains(errOut, "Update kory") || !strings.Contains(errOut, "docs.koryhub.dev") {
		t.Errorf("code %d, erreur %q", code, errOut)
	}
}

func TestUnreachableApiIsExplained(t *testing.T) {
	code, _, errOut := runKory(t, "http://127.0.0.1:1", "version")

	if code != ExitError || !strings.Contains(errOut, "kory config get api-url") {
		t.Errorf("code %d, erreur %q", code, errOut)
	}
}

func TestUnknownCommandIsAUsageError(t *testing.T) {
	code, _, errOut := runKory(t, "http://127.0.0.1:1", "publier")

	if code != ExitUsage || !strings.Contains(errOut, "publier") || !strings.Contains(errOut, "kory help") {
		t.Errorf("code %d, erreur %q", code, errOut)
	}
}

func TestConfigSetChangesTheLanguageOfMessages(t *testing.T) {
	t.Setenv("KORY_CONFIG_DIR", t.TempDir())
	t.Setenv("LANG", "en_US.UTF-8")

	var out bytes.Buffer
	if code := Run([]string{"config", "set", "lang", "fr"}, strings.NewReader(""), &out, &bytes.Buffer{}); code != ExitOK {
		t.Fatalf("config set : code %d", code)
	}

	out.Reset()
	Run([]string{"help"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if !strings.Contains(out.String(), "Commandes :") {
		t.Errorf("l'aide devrait être en français : %q", out.String())
	}
}

func TestConfigRefusesUnknownSettingsAndLanguages(t *testing.T) {
	code, _, errOut := runKory(t, "http://127.0.0.1:1", "config", "set", "lang", "de")
	if code != ExitUsage || !strings.Contains(errOut, "en, fr, ar, pt") {
		t.Errorf("langue : code %d, %q", code, errOut)
	}

	code, _, errOut = runKory(t, "http://127.0.0.1:1", "config", "get", "couleur")
	if code != ExitUsage || !strings.Contains(errOut, "lang, api-url") {
		t.Errorf("réglage : code %d, %q", code, errOut)
	}
}

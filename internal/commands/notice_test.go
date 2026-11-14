package commands

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// countingMeta : /v1/meta qui compte ses appels.
func countingMeta(t *testing.T, latest string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/meta" {
			calls.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"api_version":"v1","minimum_cli_version":"0.1.0","latest_cli_version":"` + latest + `"}`))
	}))
	t.Cleanup(server.Close)

	return server, &calls
}

func runIn(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(args, strings.NewReader(""), &out, &errOut)

	return code, out.String(), errOut.String()
}

func TestUpdateNoticeOncePerDayOnStderrNeverInstalled(t *testing.T) {
	server, calls := countingMeta(t, "9.9.9")
	t.Setenv("KORY_CONFIG_DIR", t.TempDir())
	t.Setenv("KORY_API_URL", server.URL)
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("KORY_UPDATE_CHECK", "always")

	code, out, errOut := runIn(t, "config", "get", "lang")
	if code != ExitOK || !strings.Contains(errOut, "A new version of kory is available: 9.9.9.") || strings.Contains(out, "9.9.9") {
		t.Fatalf("code %d, out %q, err %q", code, out, errOut)
	}

	// Moins de 24 h plus tard : aucun appel, aucun avis.
	_, _, errOut = runIn(t, "config", "get", "lang")
	if calls.Load() != 1 || strings.Contains(errOut, "9.9.9") {
		t.Fatalf("appels %d, err %q", calls.Load(), errOut)
	}
}

func TestUpdateNoticeSkippedOutsideATerminalWhenOffOrAfterAnError(t *testing.T) {
	server, calls := countingMeta(t, "9.9.9")
	t.Setenv("KORY_CONFIG_DIR", t.TempDir())
	t.Setenv("KORY_API_URL", server.URL)
	t.Setenv("LANG", "en_US.UTF-8")

	// Sortie redirigée (script, CI) : rien.
	t.Setenv("KORY_UPDATE_CHECK", "")
	runIn(t, "config", "get", "lang")
	// Coupé, ou commande en échec : rien.
	t.Setenv("KORY_UPDATE_CHECK", "off")
	runIn(t, "config", "get", "lang")
	t.Setenv("KORY_UPDATE_CHECK", "always")
	if code, _, _ := runIn(t, "config", "get", "inconnu"); code == ExitOK {
		t.Fatal("la commande aurait dû échouer")
	}
	if calls.Load() != 0 {
		t.Fatalf("appels : %d", calls.Load())
	}
}

func TestUpdateNoticeSilentWhenUpToDateOrUnreachable(t *testing.T) {
	server, _ := countingMeta(t, "0.0.1")
	t.Setenv("KORY_CONFIG_DIR", t.TempDir())
	t.Setenv("KORY_API_URL", server.URL)
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("KORY_UPDATE_CHECK", "always")
	if _, _, errOut := runIn(t, "config", "get", "lang"); errOut != "" {
		t.Fatalf("err %q", errOut)
	}

	t.Setenv("KORY_CONFIG_DIR", t.TempDir())
	t.Setenv("KORY_API_URL", "http://127.0.0.1:1")
	if code, _, errOut := runIn(t, "config", "get", "lang"); code != ExitOK || errOut != "" {
		t.Fatalf("code %d, err %q", code, errOut)
	}
}

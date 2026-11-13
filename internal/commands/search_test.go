package commands

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeSearchAPI : recherche, module et version, comme l'API de W05.
func fakeSearchAPI(t *testing.T) (*httptest.Server, *string) {
	t.Helper()
	lastQuery := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/search":
			lastQuery = r.URL.RawQuery
			if r.URL.Query().Get("q") == "wave senegal" {
				w.Write([]byte(`{"total":0,"results":[]}`))
				return
			}
			w.Write([]byte(`{"total":2,"results":[
				{"name":"@moussa/mobile-money-resilient","description":"Encaissement Mobile Money résistant aux coupures réseau, sans double débit.","language":"php","category":"payments","latest_version":"1.2.0","deprecated":false,"archived":false,"certified":false,"rating_avg":4.6,"rating_count":9,"download_count":1240},
				{"name":"@fatime-zakaria/sms-ar","description":"إرسال الرسائل النصية عند انقطاع الإنترنت","language":"dart","category":null,"latest_version":"0.9.2","deprecated":true,"archived":true,"certified":false,"rating_avg":null,"rating_count":0,"download_count":95}]}`))
		case "/v1/modules/moussa/mobile-money-resilient":
			w.Write([]byte(`{"name":"@moussa/mobile-money-resilient","visibility":"public","status":"active","latest_version":"1.2.0","description":"Encaissement Mobile Money.","language":"php","certified":false,"download_count":1240,
				"author":{"handle":"moussa","country":"TD"},"category":"payments","rating_avg":4.6,"rating_count":9,
				"versions":[{"version":"1.2.0","status":"published","prerelease":false,"published_at":"2026-10-01T10:00:00Z","sha256":"755fb219e14338666209ce97c70294240ced4d5bffd86b6e669c7aa48918ae16","size":1485,"deprecation_message":null},
				{"version":"1.0.1","status":"deprecated","prerelease":false,"published_at":"2026-08-01T10:00:00Z","sha256":"aa","size":1,"deprecation_message":"Même problème que 1.0.0."}]}`))
		case "/v1/modules/moussa/mobile-money-resilient/versions/1.2.0":
			w.Write([]byte(`{"name":"@moussa/mobile-money-resilient","version":"1.2.0","status":"published","prerelease":false,"license":"MIT","language":"php","published_at":null,"sha256":"755f","size":1485,"deprecation_message":null,
				"requires":[{"ecosystem":"composer","name":"php","version":">=8.3"}],
				"architecture":{"components":[{"id":"PaymentService","kind":"class","file":"src/PaymentService.php"},{"id":"PaymentGateway","kind":"interface","file":"src/Contracts/PaymentGateway.php"}],"links":[{"from":"PaymentService","to":"PaymentGateway","kind":"uses"}]},
				"files":[{"path":"kory.json","size":1,"sha256":"a"},{"path":"README.md","size":1,"sha256":"b"},{"path":"src/Contracts/PaymentGateway.php","size":1,"sha256":"c"},{"path":"src/PaymentService.php","size":1,"sha256":"d"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message":"Not found.","code":"not_found"}`))
		}
	}))
	t.Cleanup(server.Close)

	return server, &lastQuery
}

func TestSearchShowsATableWithStatusesAndFilters(t *testing.T) {
	server, lastQuery := fakeSearchAPI(t)
	s := newSession(t, server.URL)

	code, out, errOut := s.run("", "search", "mobile", "money", "--language", "php", "--limit", "5")
	if code != ExitOK {
		t.Fatalf("code %d, %q", code, errOut)
	}
	for _, want := range []string{"KORY", "@moussa/mobile-money-resilient", "1.2.0", "4.6 (9)", "1240", "sans double débit.", "0.9.2 deprecated archived", "إرسال الرسائل", "results: 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("sortie sans %q : %q", want, out)
		}
	}
	if !strings.Contains(*lastQuery, "q=mobile+money") || !strings.Contains(*lastQuery, "language=php") || !strings.Contains(*lastQuery, "limit=5") {
		t.Errorf("requête : %q", *lastQuery)
	}
}

func TestSearchWithoutResultOrWords(t *testing.T) {
	server, _ := fakeSearchAPI(t)
	s := newSession(t, server.URL)

	code, out, _ := s.run("", "search", "wave", "senegal")
	if code != ExitOK || !strings.Contains(out, `No kory for "wave senegal".`) {
		t.Errorf("aucun résultat : %d, %q", code, out)
	}
	if code, _, errOut := s.run("", "search"); code != ExitUsage || !strings.Contains(errOut, "Usage: kory search") {
		t.Errorf("sans mot : %d, %q", code, errOut)
	}
	if code, _, _ := s.run("", "search", "x", "--limit", "500"); code != ExitUsage {
		t.Errorf("limite : %d", code)
	}
}

func TestTruncateLastColumnKeepsLinesInTheTerminal(t *testing.T) {
	rows := [][]string{{"KORY", "DESCRIPTION"}, {"@a/b", strings.Repeat("é", 200)}}
	truncateLastColumn(rows, 60)
	if got := len([]rune(rows[1][1])); got != 60-2-7 || !strings.HasSuffix(rows[1][1], "…") {
		t.Errorf("largeur : %d, %q", got, rows[1][1])
	}

	untouched := [][]string{{"K", "D"}, {"a", strings.Repeat("x", 300)}}
	truncateLastColumn(untouched, 0)
	if len(untouched[1][1]) != 300 {
		t.Error("sortie redirigée : rien ne doit être tronqué")
	}
}

func TestInfoShowsEverythingBeforeInstalling(t *testing.T) {
	server, _ := fakeSearchAPI(t)
	s := newSession(t, server.URL)

	code, out, errOut := s.run("", "info", "@moussa/mobile-money-resilient")
	if code != ExitOK {
		t.Fatalf("code %d, %q", code, errOut)
	}
	for _, want := range []string{
		"@moussa/mobile-money-resilient  1.2.0", "@moussa · TD", "MIT · php · payments", "4.6 out of 5 (verified ratings: 9) · ↓ 1240",
		"Kory not certified", "1.2.0      published", "755fb2…ae16", "1.0.1      deprecated", "Même problème que 1.0.0.",
		"composer · php >=8.3", "ARCHITECTURE (components: 2, links: 1)", "PaymentService uses PaymentGateway",
		"README.md", "src/\n", "  Contracts/\n", "PaymentService.php", "Install: kory pull @moussa/mobile-money-resilient",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("sortie sans %q :\n%s", want, out)
		}
	}
	if strings.Contains(out, "kory.json") {
		t.Error("kory.json ne doit pas être listé")
	}

	code, _, errOut = s.run("", "info", "@moussa/inconnu")
	if code != ExitError || !strings.Contains(errOut, "Kory not found") {
		t.Errorf("introuvable : %d, %q", code, errOut)
	}
}

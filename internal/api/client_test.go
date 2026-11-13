package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetaSendsTheCliHeadersAndReadsTheVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/meta" {
			t.Errorf("chemin : %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "kory/") {
			t.Errorf("User-Agent : %s", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("Accept-Language") != "fr" {
			t.Errorf("Accept-Language : %s", r.Header.Get("Accept-Language"))
		}
		w.Write([]byte(`{"api_version":"v1","minimum_cli_version":"0.1.0","latest_cli_version":"0.2.0"}`))
	}))
	defer server.Close()

	meta, err := NewClient(server.URL+"/", "fr").Meta(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if meta.LatestCLIVersion != "0.2.0" || meta.APIVersion != "v1" {
		t.Errorf("meta : %+v", meta)
	}
}

func TestApiErrorsKeepTheTranslatedMessageAndCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUpgradeRequired)
		w.Write([]byte(`{"message":"Mettez à jour kory : cette version n'est plus acceptée.","code":"upgrade_required"}`))
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "fr").Meta(context.Background())

	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != CodeUpgradeRequired || apiErr.Status != 426 {
		t.Fatalf("erreur : %#v", err)
	}
	if !strings.HasPrefix(apiErr.Message, "Mettez à jour kory") {
		t.Errorf("message : %s", apiErr.Message)
	}
}

func TestUnexpectedResponsesAndNetworkFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`<html>Bad gateway</html>`))
	}))

	if _, err := NewClient(server.URL, "en").Meta(context.Background()); !errors.Is(err, ErrInvalidResponse) {
		t.Errorf("réponse HTML : %v", err)
	}

	url := server.URL
	server.Close()

	if _, err := NewClient(url, "en").Meta(context.Background()); !errors.Is(err, ErrUnreachable) {
		t.Errorf("serveur arrêté : %v", err)
	}
}

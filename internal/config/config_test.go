package config

import (
	"errors"
	"testing"
)

func TestSettingsAreSavedAndReloaded(t *testing.T) {
	t.Setenv("KORY_CONFIG_DIR", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Set("lang", "fr-FR"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Set("api-url", "https://api.koryhub.test"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Lang != "fr" || reloaded.APIURL != "https://api.koryhub.test" {
		t.Errorf("réglages relus : %+v", reloaded)
	}
}

func TestInvalidSettingsAreRefused(t *testing.T) {
	cfg := &Config{}

	cases := map[[2]string]error{
		{"lang", "de"}:              ErrInvalidLang,
		{"api-url", "pas-une-url"}:  ErrInvalidURL,
		{"api-url", "ftp://x.test"}: ErrInvalidURL,
		{"couleur", "rouge"}:        ErrUnknownKey,
	}

	for input, want := range cases {
		if err := cfg.Set(input[0], input[1]); !errors.Is(err, want) {
			t.Errorf("Set(%s, %s) = %v ; attendu %v", input[0], input[1], err, want)
		}
	}
}

func TestAPIAddressPriority(t *testing.T) {
	cfg := &Config{}
	t.Setenv(EnvAPIURL, "")

	if got := cfg.EffectiveAPIURL(); got != DefaultAPIURL {
		t.Errorf("par défaut : %s", got)
	}

	cfg.APIURL = "https://api-preprod.koryhub.dev"
	if got := cfg.EffectiveAPIURL(); got != cfg.APIURL {
		t.Errorf("réglage : %s", got)
	}

	t.Setenv(EnvAPIURL, "http://api.koryhub.test:8000")
	if got := cfg.EffectiveAPIURL(); got != "http://api.koryhub.test:8000" {
		t.Errorf("KORY_API_URL doit l'emporter : %s", got)
	}
}

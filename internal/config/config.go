// Package config lit et enregistre les réglages du CLI (TRD §18) dans le dossier
// de configuration de l'utilisateur : %AppData%\kory sous Windows, ~/.config/kory
// sous Linux, ~/Library/Application Support/kory sous macOS.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"

	"koryhub.dev/kory/internal/i18n"
)

// DefaultAPIURL est l'adresse de l'API en production.
const DefaultAPIURL = "https://api.koryhub.dev"

// EnvAPIURL remplace l'adresse de l'API (préproduction, poste local).
const EnvAPIURL = "KORY_API_URL"

// Keys liste les réglages modifiables par kory config.
var Keys = []string{"lang", "api-url", "credential-file"}

// Config contient les réglages enregistrés.
type Config struct {
	Lang           string `json:"lang,omitempty"`
	APIURL         string `json:"api_url,omitempty"`
	CredentialFile string `json:"credential_file,omitempty"`

	path string
}

// Dir renvoie le dossier de configuration de kory ; KORY_CONFIG_DIR le remplace (tests).
func Dir() (string, error) {
	if dir := os.Getenv("KORY_CONFIG_DIR"); dir != "" {
		return dir, nil
	}

	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(base, "kory"), nil
}

// Load lit les réglages ; un fichier absent donne des réglages vides.
func Load() (*Config, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}

	cfg := &Config{path: filepath.Join(dir, "config.json")}

	data, err := os.ReadFile(cfg.path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", cfg.path, err)
	}

	return cfg, nil
}

// Save enregistre les réglages, lisibles par l'utilisateur seul.
func (c *Config) Save() error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.path, append(data, '\n'), 0o600)
}

// EffectiveAPIURL renvoie l'adresse de l'API : KORY_API_URL, puis le réglage, puis la production.
func (c *Config) EffectiveAPIURL() string {
	if value := os.Getenv(EnvAPIURL); value != "" {
		return value
	}
	if c.APIURL != "" {
		return c.APIURL
	}

	return DefaultAPIURL
}

// Get renvoie la valeur d'un réglage et s'il est connu.
func (c *Config) Get(key string) (string, bool) {
	switch key {
	case "lang":
		return c.Lang, true
	case "api-url":
		return c.APIURL, true
	case "credential-file":
		return c.CredentialFile, true
	}

	return "", false
}

// ErrUnknownKey, ErrInvalidLang et ErrInvalidURL décrivent un réglage refusé.
var (
	ErrUnknownKey  = errors.New("unknown setting")
	ErrInvalidLang = errors.New("unsupported language")
	ErrInvalidURL  = errors.New("invalid address")
	ErrInvalidPath = errors.New("path must be absolute")
)

// Set change un réglage après l'avoir vérifié.
func (c *Config) Set(key, value string) error {
	switch key {
	case "lang":
		lang, ok := i18n.Parse(value)
		if !ok {
			return ErrInvalidLang
		}
		c.Lang = string(lang)
	case "api-url":
		parsed, err := url.Parse(value)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			return ErrInvalidURL
		}
		c.APIURL = value
	case "credential-file":
		if !filepath.IsAbs(value) {
			return ErrInvalidPath
		}
		c.CredentialFile = filepath.Clean(value)
	default:
		return ErrUnknownKey
	}

	return nil
}

// EffectiveCredentialFile renvoie le fichier du jeton (F-AUT-14) : le réglage
// credential-file, sinon « credentials » dans le dossier de configuration.
func (c *Config) EffectiveCredentialFile() string {
	if c.CredentialFile != "" {
		return c.CredentialFile
	}

	return filepath.Join(filepath.Dir(c.path), "credentials")
}

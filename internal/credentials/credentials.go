// Package credentials garde le jeton du CLI sur le poste (F-AUT-14, K01) : un fichier
// JSON lisible par l'utilisateur seul (droits 0600 ; sous Windows, le dossier du profil
// de l'utilisateur le protège). Le jeton est lié à l'adresse de l'API qui l'a remis.
package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotLoggedIn : aucun jeton enregistré (ou pas pour cette adresse de l'API).
var ErrNotLoggedIn = errors.New("not logged in")

// Credentials : le jeton et l'adresse de l'API qui l'a remis.
type Credentials struct {
	Token  string `json:"token"`
	APIURL string `json:"api_url"`
}

// Load lit le fichier ; absent, ou pour une autre adresse de l'API : ErrNotLoggedIn.
func Load(path, apiURL string) (*Credentials, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotLoggedIn
	}
	if err != nil {
		return nil, err
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if creds.Token == "" || !sameURL(creds.APIURL, apiURL) {
		return nil, ErrNotLoggedIn
	}

	return &creds, nil
}

// Save enregistre le jeton, lisible par l'utilisateur seul.
func Save(path string, creds Credentials) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}

	// Écriture dans un fichier voisin puis renommage : jamais de fichier à moitié écrit.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}

	return os.Rename(tmp, path)
}

// Delete efface le fichier ; absent, ce n'est pas une erreur.
func Delete(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	return nil
}

func sameURL(a, b string) bool {
	return strings.TrimRight(a, "/") == strings.TrimRight(b, "/")
}

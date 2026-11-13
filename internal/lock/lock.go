// Package lock lit et écrit kory.lock (F-INT-04, TRD §17) : pour chaque kory installé, version,
// empreinte de l'archive, dossier d'installation et empreinte de chaque fichier écrit. Le fichier
// est écrit en dernier, par un fichier temporaire puis un renommage (T-26).
package lock

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FileName : nom du fichier, à la racine du projet.
const FileName = "kory.lock"

// Version du format.
const Version = 1

// File : un fichier installé ; Kept = fichier existant gardé lors de l'arbitrage (son empreinte
// est celle du fichier du kory, pour que kory update le soumette encore à l'arbitrage).
type File struct {
	SHA256 string `json:"sha256"`
	Kept   bool   `json:"kept,omitempty"`
}

// Entry : un kory installé.
type Entry struct {
	Version     string          `json:"version"`
	SHA256      string          `json:"sha256"`
	Directory   string          `json:"directory"`
	InstalledAt string          `json:"installedAt"`
	Files       map[string]File `json:"files"`
}

// Lock : contenu de kory.lock.
type Lock struct {
	LockfileVersion int              `json:"lockfileVersion"`
	Modules         map[string]Entry `json:"modules"`
}

// Load lit kory.lock ; un fichier absent donne un verrou vide.
func Load(root string) (*Lock, error) {
	data, err := os.ReadFile(filepath.Join(root, FileName))
	if errors.Is(err, fs.ErrNotExist) {
		return &Lock{LockfileVersion: Version, Modules: map[string]Entry{}}, nil
	}
	if err != nil {
		return nil, err
	}

	var l Lock
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, err
	}
	if l.Modules == nil {
		l.Modules = map[string]Entry{}
	}

	return &l, nil
}

// Save écrit kory.lock par un fichier temporaire puis un renommage ; JSON indenté, fin LF.
func (l *Lock) Save(root string) error {
	l.LockfileVersion = Version

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(l); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(root, ".kory.lock-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), filepath.Join(root, FileName))
}

// Names : kory installés, triés.
func (l *Lock) Names() []string {
	names := make([]string, 0, len(l.Modules))
	for name := range l.Modules {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}

// Now : horodatage UTC de kory.lock et du journal.
func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// LogFile : journal local des synchronisations du projet (F-INT-11), lignes horodatées en UTC.
const LogFile = ".kory/sync.log"

// Log ajoute une ligne au journal : « date action @auteur/module version dossier ». Une erreur
// d'écriture du journal n'arrête jamais la commande.
func Log(root, action, name, version, directory string) {
	path := filepath.Join(root, filepath.FromSlash(LogFile))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	f.WriteString(strings.Join([]string{Now(), action, name, version, directory}, " ") + "\n")
}

// Package project lit et écrit les kory d'un projet (F-REG-25, TRD §18) : kory.json à la racine
// pour un projet à un seul kory, ou .kory/<nom>/kory.json et .kory/<nom>/README.md. Le manifeste
// local liste les fichiers par chemin relatif à la racine du projet (barres obliques).
package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Dir est le dossier des manifestes d'un projet à plusieurs kory ; jamais envoyé (F-REG-29).
const Dir = ".kory"

// NamePattern : nom d'un kory, comme côté serveur (minuscules, chiffres, tirets, 2 à 64).
var NamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}[a-z0-9]$`)

// ErrExists : un kory de ce nom existe déjà dans le projet.
var ErrExists = errors.New("kory already exists")

// ErrInvalidName : nom de kory refusé.
var ErrInvalidName = errors.New("invalid kory name")

// Requirement : code extérieur au kory, déclaré comme prérequis (F-REG-27, bloc C).
type Requirement struct {
	Ecosystem string `json:"ecosystem,omitempty"`
	Name      string `json:"name"`
	Version   string `json:"version,omitempty"`
}

// Manifest : kory.json local.
type Manifest struct {
	Name         string          `json:"name"`
	Version      string          `json:"version"`
	Description  string          `json:"description"`
	License      string          `json:"license"`
	Language     string          `json:"language,omitempty"`
	Tags         []string        `json:"tags"`
	Files        []string        `json:"files"`
	Requires     []Requirement   `json:"requires,omitempty"`
	Architecture json.RawMessage `json:"architecture,omitempty"`
}

// Kory : un kory du projet et l'emplacement de son manifeste et de son README.
type Kory struct {
	Manifest     Manifest
	ManifestPath string
	ReadmePath   string
}

// Discover liste les kory du projet : kory.json à la racine, puis .kory/*/kory.json, par nom.
func Discover(root string) ([]Kory, error) {
	var found []Kory

	if k, err := load(filepath.Join(root, "kory.json"), filepath.Join(root, "README.md")); err == nil {
		found = append(found, k)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	entries, err := os.ReadDir(filepath.Join(root, Dir))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, Dir, entry.Name())
		k, err := load(filepath.Join(dir, "kory.json"), filepath.Join(dir, "README.md"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		found = append(found, k)
	}

	sort.Slice(found, func(i, j int) bool { return found[i].Manifest.Name < found[j].Manifest.Name })

	return found, nil
}

// Find renvoie le kory nommé, ou le seul kory du projet si name est vide.
func Find(koryList []Kory, name string) (*Kory, bool) {
	if name == "" && len(koryList) == 1 {
		return &koryList[0], true
	}
	for i := range koryList {
		if koryList[i].Manifest.Name == name {
			return &koryList[i], true
		}
	}

	return nil, false
}

func load(manifestPath, readmePath string) (Kory, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return Kory{}, err
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Kory{}, fmt.Errorf("%s: %w", manifestPath, err)
	}

	return Kory{Manifest: m, ManifestPath: manifestPath, ReadmePath: readmePath}, nil
}

// Save écrit le manifeste (JSON indenté, fin de ligne LF).
func (k *Kory) Save() error {
	if err := os.MkdirAll(filepath.Dir(k.ManifestPath), 0o755); err != nil {
		return err
	}
	data, err := IndentJSON(k.Manifest)
	if err != nil {
		return err
	}

	return os.WriteFile(k.ManifestPath, append(data, '\n'), 0o644)
}

// InitOptions : kory init.
type InitOptions struct {
	Name    string   // vide : kory.json à la racine
	Add     []string // fichiers ou dossiers, relatifs à la racine
	License string
	Readme  string // contenu du README de départ
	// Files : liste déjà établie (kory init --from) ; sinon les fichiers de Add.
	Files []string
	// Requires : prérequis détectés (kory init --from).
	Requires []Requirement
}

// Init crée un kory : manifeste, README de départ et .koryignore par défaut s'il manque.
func Init(root string, opts InitOptions) (*Kory, error) {
	name := opts.Name
	manifestPath, readmePath := filepath.Join(root, "kory.json"), filepath.Join(root, "README.md")
	if name != "" {
		manifestPath = filepath.Join(root, Dir, name, "kory.json")
		readmePath = filepath.Join(root, Dir, name, "README.md")
	} else {
		name = SanitizeName(filepath.Base(root))
	}
	if !NamePattern.MatchString(name) {
		return nil, ErrInvalidName
	}

	if _, err := os.Stat(manifestPath); err == nil {
		return nil, ErrExists
	}

	if err := EnsureIgnoreFile(root); err != nil {
		return nil, err
	}
	ignore, err := LoadIgnore(root)
	if err != nil {
		return nil, err
	}

	files := opts.Files
	if files == nil {
		if files, err = Collect(root, opts.Add, ignore); err != nil {
			return nil, err
		}
	}
	sort.Strings(files)

	k := &Kory{
		Manifest: Manifest{
			Name:     name,
			Version:  "1.0.0",
			License:  opts.License,
			Language: DetectLanguage(files),
			Tags:     []string{},
			Files:    files,
			Requires: opts.Requires,
		},
		ManifestPath: manifestPath,
		ReadmePath:   readmePath,
	}

	if err := k.Save(); err != nil {
		return nil, err
	}
	if _, err := os.Stat(readmePath); errors.Is(err, fs.ErrNotExist) {
		if err := os.WriteFile(readmePath, []byte(opts.Readme), 0o644); err != nil {
			return nil, err
		}
	}

	return k, nil
}

// Collect parcourt les chemins donnés et renvoie les fichiers non ignorés, relatifs à la racine.
func Collect(root string, add []string, ignore *Ignore) ([]string, error) {
	seen := map[string]bool{}
	var files []string

	for _, p := range add {
		start := filepath.Join(root, filepath.FromSlash(filepath.ToSlash(p)))
		err := filepath.WalkDir(start, func(full string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, full)
			if err != nil || strings.HasPrefix(rel, "..") {
				return fmt.Errorf("%s: hors du projet", p)
			}
			rel = filepath.ToSlash(rel)
			if ignore.Match(rel) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() || seen[rel] {
				return nil
			}
			seen[rel] = true
			files = append(files, rel)

			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	sort.Strings(files)

	return files, nil
}

// DetectLanguage : langage le plus fréquent parmi les extensions connues, sinon "".
func DetectLanguage(files []string) string {
	languages := map[string]string{
		".php": "php", ".js": "javascript", ".mjs": "javascript", ".ts": "typescript", ".dart": "dart",
		".py": "python", ".go": "go", ".java": "java", ".kt": "kotlin",
	}
	counts := map[string]int{}
	best, bestCount := "", 0
	for _, f := range files {
		if lang, ok := languages[strings.ToLower(path.Ext(f))]; ok {
			counts[lang]++
			if counts[lang] > bestCount || (counts[lang] == bestCount && lang < best) {
				best, bestCount = lang, counts[lang]
			}
		}
	}

	return best
}

// SanitizeName transforme un nom de dossier en nom de kory valide.
func SanitizeName(s string) string {
	s = strings.ToLower(s)
	s = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 64 {
		s = strings.Trim(s[:64], "-")
	}

	return s
}

// IndentJSON : JSON indenté de deux espaces, sans échapper < > & (« >=8.3 » reste lisible).
func IndentJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}

	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

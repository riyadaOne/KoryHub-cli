// Package install intègre un kory téléchargé dans le projet (F-INT-01 à F-INT-05, T-26, T-27) :
// lecture sûre de l'archive (aucun « .. », chemin absolu ni lien), plan d'écriture fichier par
// fichier, écriture seulement après l'arbitrage, jamais hors du dossier du projet.
package install

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"koryhub.dev/kory/internal/lock"
)

// Limites de lecture, les mêmes que sur le serveur (W03).
const (
	MaxUncompressed = 50 << 20
	MaxFiles        = 1000
)

// ErrUnsafeArchive : archive refusée (chemin, lien, taille) ; rien n'est écrit.
var ErrUnsafeArchive = errors.New("unsafe archive")

// ErrOutsideProject : un chemin sortirait du dossier du projet.
var ErrOutsideProject = errors.New("path outside the project")

// Archive : contenu d'un kory ; kory.json n'est pas écrit dans le projet (décision W04 2a).
type Archive struct {
	Name    string
	Version string
	Files   map[string][]byte
}

// Read lit une archive .tar.gz en mémoire, avec les contrôles du serveur.
func Read(data []byte) (*Archive, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsafeArchive, err)
	}
	tr := tar.NewReader(gz)

	archive := &Archive{Files: map[string][]byte{}}
	var total int64
	var manifest []byte

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnsafeArchive, err)
		}

		name := strings.TrimPrefix(header.Name, "./")
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("%w: %s (link or special file)", ErrUnsafeArchive, name)
		}
		if !SafePath(name) {
			return nil, fmt.Errorf("%w: %s", ErrUnsafeArchive, name)
		}
		if len(archive.Files) >= MaxFiles {
			return nil, fmt.Errorf("%w: too many files", ErrUnsafeArchive)
		}

		total += header.Size
		if total > MaxUncompressed {
			return nil, fmt.Errorf("%w: too large", ErrUnsafeArchive)
		}
		content, err := io.ReadAll(io.LimitReader(tr, header.Size+1))
		if err != nil || int64(len(content)) != header.Size {
			return nil, fmt.Errorf("%w: %s", ErrUnsafeArchive, name)
		}

		if name == "kory.json" {
			manifest = content
			continue
		}
		archive.Files[name] = content
	}

	var m struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if manifest == nil || json.Unmarshal(manifest, &m) != nil {
		return nil, fmt.Errorf("%w: kory.json", ErrUnsafeArchive)
	}
	archive.Name, archive.Version = m.Name, m.Version

	return archive, nil
}

// SafePath : chemin relatif, en barres obliques, sans « .. », sans lettre de lecteur.
func SafePath(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, ":") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." || part == "." || part == "" {
			return false
		}
	}

	return path.Clean(name) == name
}

// SafeDirectory : dossier d'installation relatif au projet, sans « .. » ; « . » accepté.
func SafeDirectory(dir string) bool {
	dir = strings.TrimSuffix(filepath.ToSlash(dir), "/")

	return dir == "." || SafePath(dir)
}

// Hash : empreinte SHA-256 en hexadécimal.
func Hash(data []byte) string {
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}

// State d'un fichier du kory par rapport au projet.
type State int

const (
	New       State = iota // absent du projet
	Identical              // déjà présent, même contenu
	Conflict               // présent, différent, jamais installé par kory
	Untouched              // installé par kory, inchangé depuis : remplacé sans question
	Modified               // installé par kory puis modifié par le développeur : arbitrage
)

// Item : un fichier du kory et son état.
type Item struct {
	Path  string // relatif au dossier d'installation
	State State
	Data  []byte
}

// NeedsChoice : le fichier existe et diffère ; il ne sera remplacé qu'avec l'accord du développeur.
func (i Item) NeedsChoice() bool {
	return i.State == Conflict || i.State == Modified
}

// Plan compare les fichiers du kory au projet ; previous = entrée de kory.lock (mise à jour) ou nil.
func Plan(root, dir string, files map[string][]byte, previous *lock.Entry) ([]Item, error) {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	items := make([]Item, 0, len(paths))
	for _, p := range paths {
		target, err := Target(root, dir, p)
		if err != nil {
			return nil, err
		}

		item := Item{Path: p, Data: files[p], State: New}
		existing, err := os.ReadFile(target)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return nil, err
		case bytes.Equal(existing, files[p]):
			item.State = Identical
		case previous != nil && previous.Files[p].SHA256 != "" && !previous.Files[p].Kept && previous.Files[p].SHA256 == Hash(existing):
			item.State = Untouched
		case previous != nil && previous.Files[p].SHA256 != "":
			item.State = Modified
		default:
			item.State = Conflict
		}
		items = append(items, item)
	}

	return items, nil
}

// Target : chemin complet d'un fichier, contrôlé : dans le projet, sans dossier lien symbolique.
func Target(root, dir, file string) (string, error) {
	if !SafeDirectory(dir) || !SafePath(file) {
		return "", ErrOutsideProject
	}

	target := filepath.Join(root, filepath.FromSlash(dir), filepath.FromSlash(file))
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", ErrOutsideProject
	}

	// Aucun élément existant du chemin ne doit être un lien (il mènerait hors du projet).
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			break
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", ErrOutsideProject
		}
	}

	return target, nil
}

// Result : bilan de l'écriture et fichiers à noter dans kory.lock.
type Result struct {
	Written, Replaced, Kept, Identical int
	Files                              map[string]lock.File
}

// Write écrit les fichiers après l'arbitrage : nouveaux et inchangés depuis l'installation sont
// écrits ; un fichier à choix n'est remplacé que si replace[chemin] est vrai. Chaque fichier passe
// par un fichier temporaire puis un renommage.
func Write(root, dir string, items []Item, replace map[string]bool) (*Result, error) {
	result := &Result{Files: map[string]lock.File{}}

	for _, item := range items {
		entry := lock.File{SHA256: Hash(item.Data)}

		switch {
		case item.State == Identical:
			result.Identical++
		case item.NeedsChoice() && !replace[item.Path]:
			entry.Kept = true
			result.Kept++
		default:
			target, err := Target(root, dir, item.Path)
			if err != nil {
				return nil, err
			}
			if err := writeFile(target, item.Data); err != nil {
				return nil, err
			}
			if item.State == New {
				result.Written++
			} else {
				result.Replaced++
			}
		}
		result.Files[item.Path] = entry
	}

	return result, nil
}

func writeFile(target string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".kory-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), target)
}

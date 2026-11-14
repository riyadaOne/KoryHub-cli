package project

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// IgnoreFile : fichiers jamais envoyés (F-REG-08), à la racine du projet.
const IgnoreFile = ".koryignore"

// DefaultIgnore : liste par défaut du PRD (F-REG-08), écrite dans le .koryignore de départ.
var DefaultIgnore = []string{".env*", "*.pem", "*.key", "id_rsa*", ".git", "node_modules", "vendor"}

// toolFiles : fichiers de kory lui-même, jamais repris dans un kory (dossier des manifestes,
// liste des exclusions ; recette K02 : .koryignore était repris par kory init --add .).
var toolFiles = []string{Dir, IgnoreFile}

// Ignore applique les motifs : sans « / », le motif vise chaque élément du chemin (un dossier
// ignore tout son contenu) ; avec « / », il vise le chemin entier depuis la racine.
type Ignore struct {
	patterns []string
}

// LoadIgnore lit .koryignore (lignes vides et # ignorées) en plus de la liste par défaut.
func LoadIgnore(root string) (*Ignore, error) {
	ig := &Ignore{patterns: append(append([]string{}, DefaultIgnore...), toolFiles...)}

	f, err := os.Open(filepath.Join(root, IgnoreFile))
	if errors.Is(err, fs.ErrNotExist) {
		return ig, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ig.patterns = append(ig.patterns, strings.TrimSuffix(filepath.ToSlash(line), "/"))
	}

	return ig, scanner.Err()
}

// Match indique si le chemin relatif (barres obliques) est ignoré.
func (ig *Ignore) Match(rel string) bool {
	segments := strings.Split(rel, "/")
	for _, pattern := range ig.patterns {
		if strings.Contains(pattern, "/") {
			p := strings.TrimPrefix(pattern, "/")
			if ok, _ := path.Match(p, rel); ok || strings.HasPrefix(rel, p+"/") {
				return true
			}
			continue
		}
		for _, segment := range segments {
			if ok, _ := path.Match(pattern, segment); ok {
				return true
			}
		}
	}

	return false
}

// EnsureIgnoreFile crée .koryignore avec la liste par défaut s'il n'existe pas.
func EnsureIgnoreFile(root string) error {
	target := filepath.Join(root, IgnoreFile)
	if _, err := os.Stat(target); err == nil {
		return nil
	}

	content := "# Fichiers jamais envoyés par kory push (en plus de la liste par défaut)\n" + strings.Join(DefaultIgnore[:len(DefaultIgnore)-1], "\n") + "\n"

	return os.WriteFile(target, []byte(content), 0o644)
}

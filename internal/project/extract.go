package project

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"koryhub.dev/kory/internal/graph"
)

// MaxDepth : profondeur de suivi des liens depuis le chemin de départ (TRD §18).
const MaxDepth = 3

// Related : fichier du projet relié au kory, hors du chemin de départ, avec la raison.
type Related struct {
	Path  string // fichier proposé
	Name  string // nom qu'il déclare
	From  string // fichier du kory qui le cite
	Depth int
}

// Extract part d'un fichier ou d'un dossier (F-REG-26) : renvoie ses fichiers et les fichiers du
// projet qui y sont reliés (noms déclarés ailleurs et cités), sur MaxDepth niveaux au plus.
func Extract(root, from string, ignore *Ignore) (start []string, related []Related, err error) {
	start, err = Collect(root, []string{from}, ignore)
	if err != nil {
		return nil, nil, err
	}

	index, sources, err := indexProject(root, ignore)
	if err != nil {
		return nil, nil, err
	}

	inKory := map[string]bool{}
	for _, f := range start {
		inKory[f] = true
	}

	level := start
	for depth := 1; depth <= MaxDepth && len(level) > 0; depth++ {
		var next []string
		for _, file := range level {
			_, referenced := graph.Names(file, sources[file])
			for _, name := range referenced {
				target, ok := index[name]
				if !ok || inKory[target] {
					continue
				}
				inKory[target] = true
				related = append(related, Related{Path: target, Name: name, From: file, Depth: depth})
				next = append(next, target)
			}
		}
		level = next
	}

	sort.Slice(related, func(i, j int) bool { return related[i].Path < related[j].Path })

	return start, related, nil
}

// indexProject : nom déclaré => fichier (premier trouvé), pour les fichiers dont le langage est reconnu.
func indexProject(root string, ignore *Ignore) (map[string]string, map[string]string, error) {
	index := map[string]string{}
	sources := map[string]string{}

	err := filepath.WalkDir(root, func(full string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, full)
		rel = filepath.ToSlash(rel)
		if rel != "." && ignore.Match(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !graph.Supported(rel) {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > 1<<20 {
			return nil
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return nil
		}
		sources[rel] = string(data)
		declared, _ := graph.Names(rel, sources[rel])
		for _, name := range declared {
			if _, exists := index[name]; !exists && !strings.HasPrefix(name, "_") {
				index[name] = rel
			}
		}

		return nil
	})

	return index, sources, err
}

// ReadFiles lit les fichiers d'un kory (chemin relatif => contenu), pour l'analyse de l'architecture.
func ReadFiles(root string, files []string) map[string]string {
	out := map[string]string{}
	for _, rel := range files {
		if data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
			out[rel] = string(data)
		}
	}

	return out
}

// Shared : fichier de la liste déjà publié par un autre kory du projet (K02.9 : avertissement).
type Shared struct {
	Path string
	Kory string
}

// SharedFiles renvoie les fichiers de la liste qui figurent déjà dans un autre kory du projet
// que celui nommé (le kory en cours de création).
func SharedFiles(root string, files []string, except string) []Shared {
	others, err := Discover(root)
	if err != nil {
		return nil
	}

	owner := map[string]string{}
	for _, k := range others {
		if k.Manifest.Name == except {
			continue
		}
		for _, f := range k.Manifest.Files {
			if _, exists := owner[f]; !exists {
				owner[f] = k.Manifest.Name
			}
		}
	}

	var shared []Shared
	for _, f := range files {
		if name, ok := owner[f]; ok {
			shared = append(shared, Shared{Path: f, Kory: name})
		}
	}

	return shared
}

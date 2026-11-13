// Package graph détecte l'architecture d'un kory dans son code (F-REG-23, TRD §18, T-41) :
// composants (classes, interfaces, traits, enums ; fonctions ou fichier-module quand le langage
// n'a pas de classe) et liens entre composants du kory seulement (étend, implémente, utilise).
// Règles de lecture du code, sans programme externe ni bibliothèque en C, sans intelligence
// artificielle. Langages : PHP, JavaScript, TypeScript, Dart ; les autres arrivent après la
// démonstration (décision W03).
package graph

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

// Component : composant détecté.
type Component struct {
	ID   string `json:"id"`
	File string `json:"file"`
	Kind string `json:"kind"`
}

// Link : lien entre deux composants du kory.
type Link struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

// Architecture : ce qui est écrit dans kory.json après confirmation.
type Architecture struct {
	Components []Component `json:"components"`
	Links      []Link      `json:"links"`
}

// Result : architecture détectée et fichiers dont le langage n'est pas reconnu.
type Result struct {
	Architecture
	Unrecognized []string
}

// declaration : composant déclaré dans un fichier, avec ses liens explicites.
type declaration struct {
	name       string
	kind       string
	extends    []string
	implements []string
	uses       []string // traits (PHP), mixins (Dart)
}

// parsed : ce qu'un analyseur lit dans un fichier.
type parsed struct {
	declarations []declaration
	identifiers  map[string]bool // noms cités dans le code (hors commentaires et chaînes)
}

type analyzer func(source string) parsed

var analyzers = map[string]analyzer{
	".php": parsePHP,
	".js":  parseScript, ".mjs": parseScript, ".jsx": parseScript,
	".ts": parseScript, ".tsx": parseScript,
	".dart": parseDart,
}

// Supported indique si l'extension du fichier a un analyseur.
func Supported(file string) bool {
	_, ok := analyzers[strings.ToLower(path.Ext(file))]

	return ok
}

// Analyze lit les fichiers (chemin => contenu) et propose l'architecture.
func Analyze(files map[string]string) Result {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	type fileInfo struct {
		path   string
		parsed parsed
	}
	var infos []fileInfo
	var result Result

	for _, p := range paths {
		base := path.Base(p)
		if base == "kory.json" || strings.EqualFold(base, "README.md") {
			continue
		}
		parse, ok := analyzers[strings.ToLower(path.Ext(p))]
		if !ok {
			result.Unrecognized = append(result.Unrecognized, p)
			continue
		}
		info := fileInfo{p, parse(files[p])}
		if len(info.parsed.declarations) == 0 {
			info.parsed.declarations = []declaration{{name: moduleName(p), kind: "module"}}
		}
		infos = append(infos, info)
	}

	// Identifiants uniques : le nom, puis Nom_2, Nom_3… en cas de doublon.
	idOf := map[string]string{} // nom déclaré => identifiant (premier trouvé)
	used := map[string]int{}
	byFile := map[string][]string{}
	for _, info := range infos {
		for _, d := range info.parsed.declarations {
			used[d.name]++
			id := d.name
			if used[d.name] > 1 {
				id = fmt.Sprintf("%s_%d", d.name, used[d.name])
			}
			if _, exists := idOf[d.name]; !exists {
				idOf[d.name] = id
			}
			result.Components = append(result.Components, Component{ID: id, File: info.path, Kind: d.kind})
			byFile[info.path] = append(byFile[info.path], id)
		}
	}

	seen := map[Link]bool{}
	add := func(from, toName, kind string) {
		to, ok := idOf[toName]
		if !ok || to == from {
			return
		}
		link := Link{From: from, To: to, Kind: kind}
		if !seen[link] {
			seen[link] = true
			result.Links = append(result.Links, link)
		}
	}

	for _, info := range infos {
		ids := byFile[info.path]
		explicit := map[string]bool{}
		for i, d := range info.parsed.declarations {
			for _, name := range d.extends {
				add(ids[i], name, "extends")
				explicit[name] = true
			}
			for _, name := range d.implements {
				add(ids[i], name, "implements")
				explicit[name] = true
			}
			for _, name := range d.uses {
				add(ids[i], name, "uses")
				explicit[name] = true
			}
		}
		// Autres noms cités dans le fichier : « utilise », depuis le premier composant du fichier.
		names := make([]string, 0, len(info.parsed.identifiers))
		for name := range info.parsed.identifiers {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if !explicit[name] && !declaredIn(info.parsed.declarations, name) {
				add(ids[0], name, "uses")
			}
		}
	}

	return result
}

func declaredIn(declarations []declaration, name string) bool {
	for _, d := range declarations {
		if d.name == name {
			return true
		}
	}

	return false
}

// moduleName : nom d'un fichier-module (« payments.service.ts » → « payments.service »).
func moduleName(p string) string {
	base := path.Base(p)
	name := strings.TrimSuffix(base, path.Ext(base))
	name = regexp.MustCompile(`[^A-Za-z0-9_.-]`).ReplaceAllString(name, "_")
	if len(name) > 64 {
		name = name[:64]
	}

	return name
}

var (
	lineComment  = regexp.MustCompile(`(?m)(^|[^:\\])//[^\n]*`)
	hashComment  = regexp.MustCompile(`(?m)(^|\s)#[^\n\[]*`)
	blockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	stringLit    = regexp.MustCompile("(?s)\"(?:[^\"\\\\\\n]|\\\\.)*\"|'(?:[^'\\\\\\n]|\\\\.)*'|`(?:[^`\\\\]|\\\\.)*`")
	identifier   = regexp.MustCompile(`\b[A-Z][A-Za-z0-9_]*\b`)
)

// clean retire commentaires et chaînes, pour ne lire que le code.
func clean(source string, hash bool) string {
	s := blockComment.ReplaceAllString(source, " ")
	s = stringLit.ReplaceAllString(s, `""`)
	s = lineComment.ReplaceAllString(s, "$1")
	if hash {
		s = hashComment.ReplaceAllString(s, "$1")
	}

	return s
}

func identifiers(code string) map[string]bool {
	out := map[string]bool{}
	for _, m := range identifier.FindAllString(code, -1) {
		out[m] = true
	}

	return out
}

// names : liste « A, B\C, D » → noms courts.
func names(list string) []string {
	var out []string
	for _, part := range strings.Split(list, ",") {
		part = strings.TrimSpace(part)
		if i := strings.IndexAny(part, "<( "); i >= 0 {
			part = part[:i]
		}
		if i := strings.LastIndexAny(part, `\.`); i >= 0 {
			part = part[i+1:]
		}
		if part != "" {
			out = append(out, part)
		}
	}

	return out
}

// Names renvoie les noms déclarés dans un fichier et les noms qu'il cite (extension reconnue
// seulement) : sert à suivre les liens d'un fichier à l'autre (kory init --from, F-REG-26).
func Names(file, source string) (declared, referenced []string) {
	parse, ok := analyzers[strings.ToLower(path.Ext(file))]
	if !ok {
		return nil, nil
	}

	p := parse(source)
	own := map[string]bool{}
	for _, d := range p.declarations {
		declared = append(declared, d.name)
		own[d.name] = true
		for _, group := range [][]string{d.extends, d.implements, d.uses} {
			for _, name := range group {
				p.identifiers[name] = true
			}
		}
	}
	for name := range p.identifiers {
		if !own[name] {
			referenced = append(referenced, name)
		}
	}
	sort.Strings(referenced)

	return declared, referenced
}

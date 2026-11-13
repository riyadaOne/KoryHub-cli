package project

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Le code extérieur au projet n'est jamais inclus : il devient un prérequis (F-REG-27, TRD §18).
// Sources : composer.json (et vendor/composer/installed.json pour relier un espace de noms à son
// paquet), package.json, pubspec.yaml.

var (
	jsImport   = regexp.MustCompile(`(?m)(?:from\s+|require\(\s*|import\s+)['"]([^'"./][^'"]*)['"]`)
	dartImport = regexp.MustCompile(`(?m)import\s+['"]package:([a-z0-9_]+)/`)
	phpUse     = regexp.MustCompile(`(?m)^\s*use\s+(?:function\s+|const\s+)?\\?([A-Za-z_][\w\\]*)`)
)

// DetectRequirements propose les prérequis des fichiers d'un kory (chemins relatifs à la racine).
func DetectRequirements(root string, files []string) []Requirement {
	var found []Requirement
	seen := map[string]bool{}
	add := func(r Requirement) {
		key := r.Ecosystem + "|" + r.Name
		if !seen[key] {
			seen[key] = true
			found = append(found, r)
		}
	}

	composer := readComposer(root)
	npm := readPackageJSON(root)
	pub, pubName := readPubspec(root)

	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		content := string(data)

		switch strings.ToLower(path.Ext(rel)) {
		case ".php":
			if v, ok := composer.require["php"]; ok {
				add(Requirement{Ecosystem: "composer", Name: "php", Version: v})
			}
			for _, m := range phpUse.FindAllStringSubmatch(content, -1) {
				if pkg := composer.packageFor(m[1]); pkg != "" {
					add(Requirement{Ecosystem: "composer", Name: pkg, Version: composer.require[pkg]})
				}
			}
		case ".js", ".mjs", ".jsx", ".ts", ".tsx":
			for _, m := range jsImport.FindAllStringSubmatch(content, -1) {
				name := npmPackage(m[1])
				if v, ok := npm[name]; ok {
					add(Requirement{Ecosystem: "npm", Name: name, Version: v})
				}
			}
		case ".dart":
			for _, m := range dartImport.FindAllStringSubmatch(content, -1) {
				if v, ok := pub[m[1]]; ok && m[1] != pubName {
					add(Requirement{Ecosystem: "pub", Name: m[1], Version: v})
				}
			}
		}
	}

	sort.Slice(found, func(i, j int) bool {
		if found[i].Ecosystem != found[j].Ecosystem {
			return found[i].Ecosystem < found[j].Ecosystem
		}
		return found[i].Name < found[j].Name
	})

	return found
}

type composerInfo struct {
	require    map[string]string
	namespaces map[string]string // espace de noms (avec « \ » final) => paquet
}

// packageFor : paquet requis dont un espace de noms commence le nom donné.
func (c composerInfo) packageFor(name string) string {
	best, bestLen := "", 0
	for ns, pkg := range c.namespaces {
		if strings.HasPrefix(name+`\`, ns) && len(ns) > bestLen {
			if _, required := c.require[pkg]; required {
				best, bestLen = pkg, len(ns)
			}
		}
	}

	return best
}

func readComposer(root string) composerInfo {
	info := composerInfo{require: map[string]string{}, namespaces: map[string]string{}}

	var manifest struct {
		Require map[string]string `json:"require"`
	}
	if data, err := os.ReadFile(filepath.Join(root, "composer.json")); err == nil && json.Unmarshal(data, &manifest) == nil {
		info.require = manifest.Require
	}

	var installed struct {
		Packages []struct {
			Name     string `json:"name"`
			Autoload struct {
				PSR4 map[string]any `json:"psr-4"`
			} `json:"autoload"`
		} `json:"packages"`
	}
	if data, err := os.ReadFile(filepath.Join(root, "vendor", "composer", "installed.json")); err == nil && json.Unmarshal(data, &installed) == nil {
		for _, p := range installed.Packages {
			for ns := range p.Autoload.PSR4 {
				info.namespaces[ns] = p.Name
			}
		}
	}

	return info
}

func readPackageJSON(root string) map[string]string {
	var manifest struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	deps := map[string]string{}
	if data, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil && json.Unmarshal(data, &manifest) == nil {
		for k, v := range manifest.DevDependencies {
			deps[k] = v
		}
		for k, v := range manifest.Dependencies {
			deps[k] = v
		}
	}

	return deps
}

// readPubspec lit les dépendances de premier niveau de pubspec.yaml (lecture simple, sans YAML complet).
func readPubspec(root string) (map[string]string, string) {
	deps := map[string]string{}
	name := ""
	data, err := os.ReadFile(filepath.Join(root, "pubspec.yaml"))
	if err != nil {
		return deps, name
	}

	section := ""
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), " \r")
		if line == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") {
			key, value, _ := strings.Cut(line, ":")
			section = key
			if key == "name" {
				name = strings.TrimSpace(value)
			}
			continue
		}
		if (section == "dependencies" || section == "dev_dependencies") && strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") {
			key, value, _ := strings.Cut(strings.TrimSpace(line), ":")
			deps[key] = strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}

	return deps, name
}

// npmPackage : « @scope/nom/sous-chemin » → « @scope/nom », « nom/sous » → « nom ».
func npmPackage(spec string) string {
	parts := strings.Split(spec, "/")
	if strings.HasPrefix(spec, "@") && len(parts) > 1 {
		return parts[0] + "/" + parts[1]
	}

	return parts[0]
}

// Package archive construit l'archive .tar.gz d'un kory (TRD §13, §18, T-19, T-44) : fichiers
// relatifs à leur dossier commun, kory.json normalisé (sans chemin local), README.md ; octets
// copiés tels quels ; aucun lien ; binaires refusés sauf images ; SVG refusé ; 10 Mo au plus.
package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"koryhub.dev/kory/internal/graph"
	"koryhub.dev/kory/internal/project"
	"koryhub.dev/kory/internal/scan"
	"koryhub.dev/kory/internal/secrets"
)

// MaxSize : taille maximale d'une archive compressée (F-REG-03).
const MaxSize = 10 * 1024 * 1024

// Problem : fichier refusé avant l'envoi.
type Problem struct {
	Code string // missing, symlink, not_regular, binary, svg, conflict, too_large, readme_missing, outside
	Path string
}

// SecretsFound : secrets trouvés avant l'envoi ; la publication est bloquée, sans contournement (F-REG-07).
type SecretsFound struct {
	Findings []secrets.Finding
}

func (s *SecretsFound) Error() string {
	return fmt.Sprintf("%d secret(s)", len(s.Findings))
}

func (p *Problem) Error() string {
	return p.Code + ": " + p.Path
}

// Entry : fichier de l'archive.
type Entry struct {
	Path string
	Size int64
}

// Result : archive prête à envoyer.
type Result struct {
	Data    []byte
	SHA256  string
	Entries []Entry // fichiers du kory, sans kory.json ni README.md
	// Warnings : informations qui ressemblent à de la production (F-REG-31), chemins publiés.
	Warnings []scan.Finding
}

var images = map[string][]byte{
	".png":  []byte("\x89PNG\r\n\x1a\n"),
	".jpg":  {0xFF, 0xD8, 0xFF},
	".jpeg": {0xFF, 0xD8, 0xFF},
	".gif":  []byte("GIF8"),
	".webp": []byte("RIFF"),
}

// normalized : kory.json publié, avec le nom complet et les chemins relatifs au dossier commun.
type normalized struct {
	Name         string                `json:"name"`
	Version      string                `json:"version"`
	Description  string                `json:"description"`
	License      string                `json:"license"`
	Language     string                `json:"language,omitempty"`
	Tags         []string              `json:"tags"`
	Files        []string              `json:"files"`
	Requires     []project.Requirement `json:"requires,omitempty"`
	Architecture json.RawMessage       `json:"architecture,omitempty"`
}

// Build lit les fichiers du kory depuis root et construit l'archive publiée sous fullName (@auteur/nom).
func Build(root string, k *project.Kory, fullName string) (*Result, error) {
	files := append([]string{}, k.Manifest.Files...)
	sort.Strings(files)
	common := commonDir(files)

	type item struct {
		name string
		data []byte
	}
	var items []item
	var entries []Entry
	var published []string
	var warnings []scan.Finding
	var found []secrets.Finding

	for _, rel := range files {
		if strings.HasPrefix(rel, "/") || strings.Contains(rel, ":") || containsDotDot(rel) {
			return nil, &Problem{Code: "outside", Path: rel}
		}
		full := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Lstat(full)
		if err != nil {
			return nil, &Problem{Code: "missing", Path: rel}
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, &Problem{Code: "symlink", Path: rel}
		}
		if !info.Mode().IsRegular() {
			return nil, &Problem{Code: "not_regular", Path: rel}
		}

		data, err := os.ReadFile(full)
		if err != nil {
			return nil, err
		}
		if code := contentProblem(rel, data); code != "" {
			return nil, &Problem{Code: code, Path: rel}
		}

		name := strings.TrimPrefix(rel, common)
		if name == "kory.json" || name == "README.md" {
			return nil, &Problem{Code: "conflict", Path: rel}
		}
		warnings = append(warnings, scan.Lines(name, data)...)
		fileSecrets, err := secrets.Scan(name, data)
		if err != nil {
			return nil, err
		}
		found = append(found, fileSecrets...)
		items = append(items, item{name, data})
		entries = append(entries, Entry{Path: name, Size: int64(len(data))})
		published = append(published, name)
	}

	readme, err := os.ReadFile(k.ReadmePath)
	if err != nil {
		return nil, &Problem{Code: "readme_missing", Path: filepath.ToSlash(filepath.Base(k.ReadmePath))}
	}
	warnings = append(warnings, scan.Lines("README.md", readme)...)
	readmeSecrets, err := secrets.Scan("README.md", readme)
	if err != nil {
		return nil, err
	}
	found = append(found, readmeSecrets...)
	if len(found) > 0 {
		return nil, &SecretsFound{Findings: found}
	}

	m := k.Manifest
	tags := m.Tags
	if tags == nil {
		tags = []string{}
	}
	// Architecture : chemins des composants relatifs au dossier commun, comme les fichiers (T-44).
	architecture := m.Architecture
	if len(architecture) > 0 {
		var a graph.Architecture
		if err := json.Unmarshal(architecture, &a); err == nil {
			for i := range a.Components {
				a.Components[i].File = strings.TrimPrefix(a.Components[i].File, common)
			}
			if architecture, err = json.Marshal(a); err != nil {
				return nil, err
			}
		}
	}

	manifest, err := json.MarshalIndent(normalized{
		Name: fullName, Version: m.Version, Description: m.Description, License: m.License, Language: m.Language,
		Tags: tags, Files: published, Requires: m.Requires, Architecture: architecture,
	}, "", "  ")
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	all := append([]item{{"kory.json", append(manifest, '\n')}, {"README.md", readme}}, items...)
	for _, it := range all {
		header := &tar.Header{Name: it.name, Mode: 0o644, Size: int64(len(it.data)), ModTime: time.Unix(0, 0), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(header); err != nil {
			return nil, err
		}
		if _, err := tw.Write(it.data); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}

	if buf.Len() > MaxSize {
		return nil, &Problem{Code: "too_large", Path: fmt.Sprintf("%d", buf.Len())}
	}

	sum := sha256.Sum256(buf.Bytes())

	return &Result{Data: buf.Bytes(), SHA256: hex.EncodeToString(sum[:]), Entries: entries, Warnings: warnings}, nil
}

// contentProblem : binaire (hors images de la documentation) ou SVG (piège 21).
func contentProblem(rel string, data []byte) string {
	ext := strings.ToLower(path.Ext(rel))
	head := data
	if len(head) > 8192 {
		head = head[:8192]
	}

	if ext == ".svg" || bytes.HasPrefix(bytes.TrimSpace(head), []byte("<svg")) {
		return "svg"
	}
	if magic, ok := images[ext]; ok && bytes.HasPrefix(head, magic) {
		return ""
	}
	if bytes.IndexByte(head, 0) >= 0 {
		return "binary"
	}

	return ""
}

// commonDir : dossier commun des fichiers, avec la barre finale (« src/Payments/ »), ou "".
func commonDir(files []string) string {
	if len(files) == 0 {
		return ""
	}
	prefix := path.Dir(files[0])
	for _, f := range files[1:] {
		for prefix != "." && !strings.HasPrefix(path.Dir(f)+"/", prefix+"/") {
			prefix = path.Dir(prefix)
		}
	}
	if prefix == "." || prefix == "/" {
		return ""
	}

	return prefix + "/"
}

func containsDotDot(rel string) bool {
	for _, segment := range strings.Split(rel, "/") {
		if segment == ".." {
			return true
		}
	}

	return false
}

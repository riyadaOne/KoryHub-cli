// Package secrets cherche les secrets avant l'envoi (F-REG-07, TRD §18, §22) avec les règles par
// défaut de gitleaks (rules.json, exportées de gitleaks v8.30.1, licence MIT : voir NOTICE) et la
// règle des jetons kory_, appliquées avec la bibliothèque standard (décision W03 bloc B, option A).
// Le serveur repasse le vrai gitleaks : c'est lui qui fait foi.
package secrets

import (
	_ "embed"
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
)

//go:embed rules.json
var rulesJSON []byte

// Finding : un secret trouvé ; jamais sa valeur.
type Finding struct {
	Path string
	Line int
	Rule string
}

type allowlist struct {
	Condition   string   `json:"condition"`
	RegexTarget string   `json:"regex_target"`
	Regexes     []string `json:"regexes"`
	Paths       []string `json:"paths"`
	StopWords   []string `json:"stopwords"`

	regexes []*regexp.Regexp
	paths   []*regexp.Regexp
}

type rule struct {
	ID          string      `json:"id"`
	Regex       string      `json:"regex"`
	Path        string      `json:"path"`
	SecretGroup int         `json:"secret_group"`
	Entropy     float64     `json:"entropy"`
	Keywords    []string    `json:"keywords"`
	Allowlists  []allowlist `json:"allowlists"`

	regex *regexp.Regexp
	path  *regexp.Regexp
}

type ruleset struct {
	Rules      []rule      `json:"rules"`
	Allowlists []allowlist `json:"allowlists"`
}

var (
	loaded  *ruleset
	loadErr error
	once    sync.Once
)

// koryToken : jetons du CLI KoryHub (TRD §7.2), comme côté serveur.
var koryToken = rule{ID: "koryhub-cli-token", Regex: `kory_[A-Za-z0-9]{40}[0-9a-f]{8}`, Keywords: []string{"kory_"}}

func rules() (*ruleset, error) {
	once.Do(func() {
		var rs ruleset
		if loadErr = json.Unmarshal(rulesJSON, &rs); loadErr != nil {
			return
		}
		rs.Rules = append(rs.Rules, koryToken)

		valid := rs.Rules[:0]
		for _, r := range rs.Rules {
			if r.Regex != "" {
				compiled, err := regexp.Compile(r.Regex)
				if err != nil {
					continue // règle écrite pour un autre moteur : ignorée, le serveur la vérifie
				}
				r.regex = compiled
			}
			if r.Path != "" {
				if compiled, err := regexp.Compile(r.Path); err == nil {
					r.path = compiled
				}
			}
			for i := range r.Allowlists {
				compileAllowlist(&r.Allowlists[i])
			}
			for i, k := range r.Keywords {
				r.Keywords[i] = strings.ToLower(k)
			}
			valid = append(valid, r)
		}
		rs.Rules = valid
		for i := range rs.Allowlists {
			compileAllowlist(&rs.Allowlists[i])
		}
		loaded = &rs
	})

	return loaded, loadErr
}

func compileAllowlist(a *allowlist) {
	for _, expr := range a.Regexes {
		if compiled, err := regexp.Compile(expr); err == nil {
			a.regexes = append(a.regexes, compiled)
		}
	}
	for _, expr := range a.Paths {
		if compiled, err := regexp.Compile(expr); err == nil {
			a.paths = append(a.paths, compiled)
		}
	}
	for i, w := range a.StopWords {
		a.StopWords[i] = strings.ToLower(w)
	}
}

// Scan cherche les secrets dans un fichier (chemin publié, contenu exact).
func Scan(path string, content []byte) ([]Finding, error) {
	rs, err := rules()
	if err != nil {
		return nil, err
	}
	for _, a := range rs.Allowlists {
		if len(a.paths) > 0 && matchAny(a.paths, path) && len(a.regexes) == 0 && len(a.StopWords) == 0 {
			return nil, nil
		}
	}

	text := string(content)
	lower := strings.ToLower(text)
	seen := map[Finding]bool{}
	var findings []Finding

	for i := range rs.Rules {
		r := &rs.Rules[i]
		if r.path != nil && !r.path.MatchString(path) {
			continue
		}
		if r.regex == nil {
			if r.path != nil {
				findings = appendOnce(findings, seen, Finding{Path: path, Line: 1, Rule: r.ID})
			}
			continue
		}
		if len(r.Keywords) > 0 && !containsAny(lower, r.Keywords) {
			continue
		}

		for _, idx := range r.regex.FindAllStringSubmatchIndex(text, -1) {
			match := text[idx[0]:idx[1]]
			secret := secretOf(text, idx, r.SecretGroup)
			if r.Entropy > 0 && entropy(secret) < r.Entropy {
				continue
			}
			line := lineAt(text, idx[0])
			if allowed(r.Allowlists, path, secret, match, line) || allowed(rs.Allowlists, path, secret, match, line) {
				continue
			}
			findings = appendOnce(findings, seen, Finding{Path: path, Line: strings.Count(text[:idx[0]], "\n") + 1, Rule: r.ID})
		}
	}

	sort.Slice(findings, func(i, j int) bool { return findings[i].Line < findings[j].Line })

	return findings, nil
}

// secretOf : groupe désigné par la règle, sinon premier groupe non vide, sinon toute la correspondance.
func secretOf(text string, idx []int, group int) string {
	if group > 0 && 2*group+1 < len(idx) && idx[2*group] >= 0 {
		return text[idx[2*group]:idx[2*group+1]]
	}
	if group == 0 {
		for g := 1; 2*g+1 < len(idx); g++ {
			if idx[2*g] >= 0 && idx[2*g+1] > idx[2*g] {
				return text[idx[2*g]:idx[2*g+1]]
			}
		}
	}

	return text[idx[0]:idx[1]]
}

func allowed(lists []allowlist, path, secret, match, line string) bool {
	for _, a := range lists {
		var checks []bool
		if len(a.paths) > 0 {
			checks = append(checks, matchAny(a.paths, path))
		}
		if len(a.regexes) > 0 {
			target := secret
			switch a.RegexTarget {
			case "match":
				target = match
			case "line":
				target = line
			}
			checks = append(checks, matchAny(a.regexes, target))
		}
		if len(a.StopWords) > 0 {
			checks = append(checks, containsAny(strings.ToLower(secret), a.StopWords))
		}
		if len(checks) == 0 {
			continue
		}

		all, any := true, false
		for _, ok := range checks {
			all = all && ok
			any = any || ok
		}
		if (a.Condition == "AND" && all) || (a.Condition != "AND" && any) {
			return true
		}
	}

	return false
}

func lineAt(text string, offset int) string {
	start := strings.LastIndex(text[:offset], "\n") + 1
	end := strings.Index(text[offset:], "\n")
	if end < 0 {
		return text[start:]
	}

	return text[start : offset+end]
}

func matchAny(list []*regexp.Regexp, s string) bool {
	for _, re := range list {
		if re.MatchString(s) {
			return true
		}
	}

	return false
}

func containsAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}

	return false
}

func appendOnce(list []Finding, seen map[Finding]bool, f Finding) []Finding {
	if seen[f] {
		return list
	}
	seen[f] = true

	return append(list, f)
}

// entropy : entropie de Shannon du secret, comme gitleaks.
func entropy(s string) float64 {
	if s == "" {
		return 0
	}
	counts := map[rune]int{}
	for _, r := range s {
		counts[r]++
	}
	total := float64(len([]rune(s)))
	var e float64
	for _, c := range counts {
		p := float64(c) / total
		e -= p * math.Log2(p)
	}

	return e
}

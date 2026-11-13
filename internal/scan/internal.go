// Package scan repère, avant l'envoi, ce qui ressemble à une information interne de production
// (F-REG-31, TRD §18) : adresses web, adresses IP, noms de domaine, adresses e-mail, chaînes de
// connexion, noms de base de données. Avertissement seulement : l'auteur remplace la valeur par
// un exemple ou confirme. Les valeurs d'exemple et de documentation sont ignorées.
package scan

import (
	"bufio"
	"bytes"
	"net"
	"net/url"
	"regexp"
	"strings"
)

// Kind : type d'information repérée.
type Kind string

const (
	KindURL        Kind = "url"
	KindIP         Kind = "ip"
	KindDomain     Kind = "domain"
	KindEmail      Kind = "email"
	KindConnection Kind = "connection"
	KindDatabase   Kind = "database"
)

// Finding : une ligne à relire.
type Finding struct {
	Path  string
	Line  int
	Kind  Kind
	Value string
}

var (
	connectionRe = regexp.MustCompile(`(?i)\b(?:mysql|mariadb|postgres(?:ql)?|mongodb(?:\+srv)?|redis|rediss|amqps?|sqlserver|mssql|oracle|jdbc:[a-z]+)://[^\s'"<>` + "`" + `]+`)
	urlRe        = regexp.MustCompile(`(?i)\bhttps?://[^\s'"<>()` + "`" + `]+`)
	emailRe      = regexp.MustCompile(`(?i)\b[a-z0-9._%+-]+@([a-z0-9-]+\.)+[a-z]{2,}\b`)
	ipv4Re       = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	ipv6Re       = regexp.MustCompile(`(?i)\b(?:[0-9a-f]{1,4}:){2,7}[0-9a-f]{1,4}\b`)
	quotedHostRe = regexp.MustCompile(`["'](([a-z0-9-]+\.)+[a-z]{2,})["']`)
	databaseRe   = regexp.MustCompile(`(?i)(?:\bDB_DATABASE|\bDATABASE_NAME|\bDB_NAME|["']database["'])\s*(?:=>|=|:)\s*["']?([A-Za-z0-9_.-]+)`)
)

// Suffixes qui sont des extensions de fichier, pas des noms de domaine (« PaymentService.php »).
var fileSuffixes = map[string]bool{
	"php": true, "js": true, "ts": true, "go": true, "py": true, "java": true, "kt": true, "dart": true, "json": true,
	"md": true, "html": true, "css": true, "txt": true, "xml": true, "yml": true, "yaml": true, "lock": true, "env": true,
	"sql": true, "sh": true, "png": true, "jpg": true, "jpeg": true, "gif": true, "webp": true, "vue": true, "tsx": true, "jsx": true,
}

var placeholderDatabases = map[string]bool{
	"example": true, "test": true, "testing": true, "database": true, "db": true, "your_database": true, "my_database": true,
	"dbname": true, "database_name": true, "app": true, "kory": true,
}

// Lines repère les informations internes dans le contenu d'un fichier.
func Lines(path string, content []byte) []Finding {
	var findings []Finding
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for number := 1; scanner.Scan(); number++ {
		line := scanner.Text()
		seen := map[string]bool{}
		add := func(kind Kind, value string) {
			if !seen[value] {
				seen[value] = true
				findings = append(findings, Finding{Path: path, Line: number, Kind: kind, Value: value})
			}
		}

		for _, m := range connectionRe.FindAllString(line, -1) {
			add(KindConnection, m)
		}
		for _, m := range urlRe.FindAllString(line, -1) {
			if !connectionRe.MatchString(m) && !ignoredHost(hostOf(m)) {
				add(KindURL, strings.TrimRight(m, ".,;"))
			}
		}
		for _, m := range emailRe.FindAllString(line, -1) {
			if !ignoredHost(m[strings.LastIndex(m, "@")+1:]) && !strings.Contains(line, "://"+m) {
				add(KindEmail, m)
			}
		}
		for _, m := range ipv4Re.FindAllString(line, -1) {
			if ip := net.ParseIP(m); ip != nil && !ignoredIP(ip) {
				add(KindIP, m)
			}
		}
		for _, m := range ipv6Re.FindAllString(line, -1) {
			if ip := net.ParseIP(m); ip != nil && !ignoredIP(ip) {
				add(KindIP, m)
			}
		}
		for _, m := range quotedHostRe.FindAllStringSubmatch(line, -1) {
			host := strings.ToLower(m[1])
			suffix := host[strings.LastIndex(host, ".")+1:]
			if !fileSuffixes[suffix] && !ignoredHost(host) && !strings.Contains(line, "://"+host) && !strings.Contains(line, "@"+host) {
				add(KindDomain, host)
			}
		}
		for _, m := range databaseRe.FindAllStringSubmatch(line, -1) {
			if !placeholderDatabases[strings.ToLower(m[1])] && !strings.HasPrefix(m[1], "$") {
				add(KindDatabase, m[1])
			}
		}
	}

	return findings
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	return strings.ToLower(u.Hostname())
}

// ignoredHost : domaines d'exemple et de test (TRD §18).
func ignoredHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" || host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ignoredIP(ip)
	}
	for _, reserved := range []string{"example.com", "example.org", "example.net"} {
		if host == reserved || strings.HasSuffix(host, "."+reserved) {
			return true
		}
	}

	return strings.HasSuffix(host, ".test") || strings.HasSuffix(host, ".example") || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".invalid")
}

var documentation = []string{"192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24", "2001:db8::/32"}

// ignoredIP : boucle locale, adresse nulle, plages réservées à la documentation.
func ignoredIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsUnspecified() {
		return true
	}
	for _, cidr := range documentation {
		_, block, _ := net.ParseCIDR(cidr)
		if block.Contains(ip) {
			return true
		}
	}

	return false
}

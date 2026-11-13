package commands

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"koryhub.dev/kory/internal/credentials"
	"koryhub.dev/kory/internal/i18n"
)

// now est remplacé dans les tests.
var now = time.Now

// loadCredentials lit le jeton de ce terminal pour l'API en cours ; un fichier illisible
// est signalé ici (ok = false), un terminal non relié renvoie (nil, true).
func loadCredentials(e *env) (*credentials.Credentials, bool) {
	creds, err := credentials.Load(e.config.EffectiveCredentialFile(), e.config.EffectiveAPIURL())
	if errors.Is(err, credentials.ErrNotLoggedIn) {
		return nil, true
	}
	if err != nil {
		e.ui.Error(i18n.T("auth.unreadable", err.Error()), i18n.T("auth.unreadable_action"))

		return nil, false
	}

	return creds, true
}

// printDetails écrit les lignes « libellé  valeur » alignées sous un message.
func printDetails(e *env, rows [][2]string) {
	width := 0
	for _, row := range rows {
		width = max(width, utf8.RuneCountInString(row[0]))
	}

	for _, row := range rows {
		padding := strings.Repeat(" ", width-utf8.RuneCountInString(row[0]))
		e.ui.Println("  " + row[0] + padding + "  " + row[1])
	}
}

// expiryText : « 2027-02-11 (dans 90 jours) », ou « jamais » pour un jeton sans expiration.
func expiryText(expiresAt *string) string {
	if expiresAt == nil || *expiresAt == "" {
		return i18n.T("report.never")
	}

	at, err := time.Parse(time.RFC3339, *expiresAt)
	if err != nil {
		return *expiresAt
	}

	date := at.Local().Format("2006-01-02")
	days := int(math.Floor(at.Sub(now()).Hours() / 24))
	if days < 1 {
		return i18n.T("report.expires_soon", date)
	}

	return i18n.T("report.expires_in", date, days)
}

// siteURL déduit l'adresse du site de celle de l'API (api.koryhub.dev → koryhub.dev).
func siteURL(apiURL string) string {
	parsed, err := url.Parse(apiURL)
	if err != nil || parsed.Host == "" {
		return apiURL
	}

	parsed.Host = strings.TrimPrefix(parsed.Host, "api.")
	parsed.Path = ""

	return strings.TrimRight(parsed.String(), "/")
}

func formatHandle(handle string) string {
	return fmt.Sprintf("@%s", handle)
}

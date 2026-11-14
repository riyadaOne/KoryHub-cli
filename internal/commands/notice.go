package commands

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"koryhub.dev/kory/internal/config"
	"koryhub.dev/kory/internal/i18n"
	"koryhub.dev/kory/internal/semver"
	"koryhub.dev/kory/internal/version"
)

// noticeInterval : l'avis de nouvelle version s'affiche au plus une fois par période (K05, décision 4a).
const noticeInterval = 24 * time.Hour

// noticeState est gardé dans le dossier de configuration (update-check.json).
type noticeState struct {
	CheckedAt time.Time `json:"checked_at"`
}

// maybeNotifyUpdate signale une nouvelle version de kory après une commande réussie : au plus une
// fois par 24 h, sur la sortie d'erreur, seulement dans un terminal (jamais dans un script ou la CI),
// jamais installée (P3). KORY_UPDATE_CHECK=off la coupe ; =always la force (tests). Toute erreur
// (réseau, fichier) est ignorée : l'avis ne bloque jamais.
func maybeNotifyUpdate(e *env, command string) {
	mode := os.Getenv("KORY_UPDATE_CHECK")
	if mode == "off" || command == "version" || command == "help" {
		return
	}
	if mode != "always" && !e.ui.Terminal() {
		return
	}

	dir, err := config.Dir()
	if err != nil {
		return
	}
	path := filepath.Join(dir, "update-check.json")
	var state noticeState
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &state)
	}
	if time.Since(state.CheckedAt) < noticeInterval {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	meta, err := e.client().Meta(ctx)
	if err != nil {
		return
	}

	if data, err := json.Marshal(noticeState{CheckedAt: time.Now()}); err == nil {
		_ = os.MkdirAll(dir, 0o700)
		_ = os.WriteFile(path, data, 0o600)
	}

	current, errCurrent := semver.Parse(version.Version)
	latest, errLatest := semver.Parse(meta.LatestCLIVersion)
	if errCurrent == nil && errLatest == nil && current.Compare(latest) < 0 {
		e.ui.Notice(i18n.T("version.update_available", latest.String()), i18n.T("version.update_action"))
	}
}

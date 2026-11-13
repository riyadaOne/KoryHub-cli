package commands

import (
	"os"

	"koryhub.dev/kory/internal/api"
	"koryhub.dev/kory/internal/i18n"
	"koryhub.dev/kory/internal/lock"
)

// runAudit contrôle les versions de kory.lock (K03.5, F-INT-13) : à jour, mise à jour,
// dépréciée, retirée, introuvable. Code de sortie 1 si une version est retirée (ou à risque, W07),
// pour l'intégration continue. Rien n'est enregistré sur le serveur.
func runAudit(e *env, args []string) int {
	if len(args) > 0 {
		e.ui.Error(i18n.T("audit.usage"), "")
		return ExitUsage
	}

	root, err := os.Getwd()
	if err != nil {
		e.ui.Error(err.Error(), "")
		return ExitError
	}
	l, err := lock.Load(root)
	if err != nil {
		e.ui.Error(i18n.T("lock.unreadable", err.Error()), i18n.T("lock.unreadable_action"))
		return ExitError
	}
	if len(l.Modules) == 0 {
		e.ui.Println("  " + i18n.T("audit.empty"))
		return ExitOK
	}

	creds, loaded := loadCredentials(e)
	if !loaded {
		return ExitError
	}
	token := ""
	if creds != nil {
		token = creds.Token
	}

	ctx, cancel := requestContext()
	defer cancel()
	results, err := e.client().Audit(ctx, token, auditEntries(l, ""))
	if err != nil {
		return reportAPIError(e, err)
	}

	rows := [][]string{{i18n.T("audit.col_module"), i18n.T("audit.col_version"), i18n.T("audit.col_state"), i18n.T("audit.col_action")}}
	var notes []string
	blocking := 0
	for _, r := range results {
		action := "—"
		switch r.State {
		case "update_available":
			action = "kory update " + r.Name
		case "deprecated":
			if r.LatestVersion != nil && newer(*r.LatestVersion, r.Version) {
				action = i18n.T("audit.update_to", r.Name, *r.LatestVersion)
			}
		case "removed", "at_risk", "not_found":
			action = "kory remove " + r.Name
		}
		state := i18n.T("audit.state." + r.State)
		if r.State == "update_available" && r.LatestVersion != nil {
			state = i18n.T("audit.state.update_to", *r.LatestVersion)
		}
		rows = append(rows, []string{r.Name, r.Version, state, action})
		if r.Message != nil && *r.Message != "" {
			notes = append(notes, r.Name+" "+r.Version+" : "+*r.Message)
		}
		if r.State == "removed" || r.State == "at_risk" {
			blocking++
		}
	}
	printTable(e, rows)
	for _, n := range notes {
		e.ui.Println("    " + n)
	}

	if blocking > 0 {
		e.ui.Error(i18n.T("audit.blocking", blocking), i18n.T("audit.blocking_action"))
		return ExitError
	}
	e.ui.Success(i18n.T("audit.done", len(results)))

	return ExitOK
}

func auditEntries(l *lock.Lock, except string) []api.AuditEntry {
	var entries []api.AuditEntry
	for _, name := range l.Names() {
		if name != except {
			entries = append(entries, api.AuditEntry{Name: name, Version: l.Modules[name].Version})
		}
	}

	return entries
}

// notifyOtherUpdates : après pull ou update, rappelle les autres kory à mettre à jour, selon le
// réglage update-notify (F-INT-07) ; une erreur réseau est ignorée.
func notifyOtherUpdates(e *env, token string, l *lock.Lock, except string) {
	level := e.config.EffectiveUpdateNotify()
	entries := auditEntries(l, except)
	if level == "none" || len(entries) == 0 {
		return
	}

	ctx, cancel := requestContext()
	defer cancel()
	results, err := e.client().Audit(ctx, token, entries)
	if err != nil {
		return
	}

	count := 0
	for _, r := range results {
		switch r.State {
		case "deprecated", "removed", "at_risk":
			count++
		case "update_available":
			if level == "all" {
				count++
			}
		}
	}
	if count > 0 {
		e.ui.Warn(i18n.T("notify.others", count), i18n.T("notify.others_action"))
	}
}

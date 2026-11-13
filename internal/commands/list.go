package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"koryhub.dev/kory/internal/credentials"
	"koryhub.dev/kory/internal/i18n"
	"koryhub.dev/kory/internal/project"
)

// runList montre les kory du projet (K02.2, F-REG-28) : nom local, nom publié, dernière
// version publiée (si le terminal est relié), nombre de fichiers.
func runList(e *env, args []string) int {
	if len(args) > 0 {
		e.ui.Error(i18n.T("error.unknown_command", "list "+args[0]), i18n.T("error.unknown_command_hint"))

		return ExitUsage
	}

	root, err := os.Getwd()
	if err != nil {
		e.ui.Error(err.Error(), "")

		return ExitError
	}
	koryList, err := project.Discover(root)
	if err != nil {
		e.ui.Error(i18n.T("project.unreadable", err.Error()), "")

		return ExitError
	}
	if len(koryList) == 0 {
		e.ui.Println(i18n.T("list.empty"))

		return ExitOK
	}

	handle := currentHandle(e)
	creds, _ := credentials.Load(e.config.EffectiveCredentialFile(), e.config.EffectiveAPIURL())

	rows := [][]string{{i18n.T("list.col_kory"), i18n.T("list.col_published"), i18n.T("list.col_version"), i18n.T("list.col_files")}}
	for _, k := range koryList {
		published, latest := "—", "—"
		if handle != "" {
			published = "@" + handle + "/" + k.Manifest.Name
			ctx, cancel := requestContext()
			if m, err := e.client().GetModule(ctx, creds.Token, handle, k.Manifest.Name); err == nil && m != nil && m.LatestVersion != nil {
				latest = *m.LatestVersion
			}
			cancel()
		}
		rows = append(rows, []string{k.Manifest.Name, published, latest, fmt.Sprint(len(k.Manifest.Files))})
	}

	printTable(e, rows)

	return ExitOK
}

func printTable(e *env, rows [][]string) {
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], len([]rune(cell)))
		}
	}
	for _, row := range rows {
		var line strings.Builder
		line.WriteString("  ")
		for i, cell := range row {
			line.WriteString(cell)
			if i < len(row)-1 {
				line.WriteString(strings.Repeat(" ", widths[i]-len([]rune(cell))+3))
			}
		}
		e.ui.Println(strings.TrimRight(line.String(), " "))
	}
}

// currentHandle renvoie le nom @auteur du compte relié, ou "" (pas relié, hors ligne).
func currentHandle(e *env) string {
	creds, err := credentials.Load(e.config.EffectiveCredentialFile(), e.config.EffectiveAPIURL())
	if err != nil {
		return ""
	}
	ctx, cancel := requestContext()
	defer cancel()

	user, err := e.client().CurrentUser(ctx, creds.Token)
	if err != nil {
		return ""
	}

	return user.Handle
}

func relative(root, full string) string {
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return full
	}

	return rel
}

package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"koryhub.dev/kory/internal/api"
	"koryhub.dev/kory/internal/i18n"
	"koryhub.dev/kory/internal/install"
	"koryhub.dev/kory/internal/lock"
)

// runPull installe un kory dans le projet (K03.1, K03.2, F-INT-01 à F-INT-05, F-INT-08, F-INT-09) :
// kory pull @auteur/module[@version] [--dir <dossier>] [--yes].
func runPull(e *env, args []string) int {
	var target spec
	var parsed bool
	dir := ""
	assumeYes := false

	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--dir" && i+1 < len(args):
			dir = args[i+1]
			i++
		case arg == "--yes" || arg == "-y":
			assumeYes = true
		case !strings.HasPrefix(arg, "-") && !parsed:
			target, parsed = parseSpec(arg)
			if !parsed {
				e.ui.Error(i18n.T("pull.invalid_name", arg), i18n.T("pull.usage"))
				return ExitUsage
			}
		default:
			e.ui.Error(i18n.T("pull.usage"), "")
			return ExitUsage
		}
	}
	if !parsed {
		e.ui.Error(i18n.T("pull.usage"), "")
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
	if installed, ok := l.Modules[target.fullName()]; ok {
		e.ui.Success(i18n.T("pull.already_installed", target.fullName(), installed.Version, filepath.FromSlash(installed.Directory)))
		e.ui.Println("  " + i18n.T("pull.update_hint", target.fullName()))
		return ExitOK
	}

	// Compte facultatif : le téléchargement est alors relié au compte (F-RCH-14) et la limite monte.
	creds, loaded := loadCredentials(e)
	if !loaded {
		return ExitError
	}
	token := ""
	if creds != nil {
		token = creds.Token
	}

	module, detail, code, ok := resolve(e, token, target)
	if !ok {
		return code
	}

	printHeader(e, module, detail)
	printWarnings(e, module, detail)
	printRequirements(e, detail)
	if code, ok := confirm(e, i18n.T("pull.continue"), assumeYes); !ok {
		return code
	}

	if dir == "" {
		dir = "kory/" + target.Name
		if answer, ok := e.ui.Prompt(i18n.T("pull.directory", dir)); ok && answer != "" {
			dir = answer
		}
	}
	dir = strings.TrimSuffix(filepath.ToSlash(dir), "/")
	if !install.SafeDirectory(dir) {
		e.ui.Error(i18n.T("pull.directory_invalid", dir), i18n.T("pull.directory_action"))
		return ExitUsage
	}

	archive, ok := download(e, root, token, target, detail)
	if !ok {
		return ExitError
	}

	items, err := install.Plan(root, dir, archive.Files, nil)
	if err != nil {
		e.ui.Error(i18n.T("pull.directory_invalid", dir), i18n.T("pull.directory_action"))
		return ExitError
	}
	replace := arbitrate(e, root, dir, items)

	if !apply(e, root, "pull", l, target.fullName(), detail, dir, items, replace) {
		return ExitError
	}
	notifyOtherUpdates(e, token, l, target.fullName())

	return ExitOK
}

// resolve lit le module et choisit la version : demandée, sinon la dernière stable.
func resolve(e *env, token string, target spec) (*api.Module, *api.VersionDetail, int, bool) {
	ctx, cancel := requestContext()
	defer cancel()

	module, err := e.client().GetModule(ctx, token, target.Author, target.Name)
	if err != nil {
		return nil, nil, reportAPIError(e, err), false
	}
	if module == nil {
		e.ui.Error(i18n.T("pull.not_found", target.fullName()), i18n.T("pull.not_found_action"))
		return nil, nil, ExitError, false
	}

	number := target.Version
	if number == "" {
		if module.LatestVersion == nil {
			e.ui.Error(i18n.T("pull.no_stable", target.fullName()), i18n.T("pull.no_stable_action", target.fullName()))
			return nil, nil, ExitError, false
		}
		number = *module.LatestVersion
	}

	detail, err := e.client().GetVersion(ctx, token, target.Author, target.Name, number)
	var apiErr *api.Error
	if errors.As(err, &apiErr) && apiErr.Status == 404 {
		e.ui.Error(i18n.T("pull.version_not_found", target.fullName(), number), i18n.T("pull.not_found_action"))
		return nil, nil, ExitError, false
	}
	if err != nil {
		return nil, nil, reportAPIError(e, err), false
	}
	if detail.Status == "removed" {
		// Le serveur refuse le téléchargement (410) ; le message vient de lui.
		err := e.client().Download(context.Background(), token, target.Author, target.Name, number, discard{}, nil)
		if err != nil {
			reportDownloadError(e, err)
		}
		e.ui.Println("  " + i18n.T("pull.removed_action", target.fullName()))
		return nil, nil, ExitError, false
	}

	return module, detail, ExitOK, true
}

func printHeader(e *env, module *api.Module, detail *api.VersionDetail) {
	parts := []string{detail.Version, detail.License}
	if detail.Language != nil && *detail.Language != "" {
		parts = append(parts, *detail.Language)
	}
	// Même compte que la fiche du site : sans kory.json ni README.md.
	count := 0
	for _, f := range detail.Files {
		if f.Path != "kory.json" && f.Path != "README.md" {
			count++
		}
	}
	parts = append(parts, i18n.T("pull.files_count", count))
	e.ui.Println(fmt.Sprintf("  %s %s", module.Name, strings.Join(parts, " · ")))
	if module.Description != nil && *module.Description != "" {
		e.ui.Println("  " + *module.Description)
	}
}

// printRequirements : code extérieur à installer soi-même ; KoryHub n'installe rien (F-REG-27).
func printRequirements(e *env, detail *api.VersionDetail) {
	if len(detail.Requires) == 0 {
		return
	}
	e.ui.Println("  " + i18n.T("pull.requirements"))
	for _, r := range detail.Requires {
		line := r.Name
		if r.Ecosystem != nil && *r.Ecosystem != "" {
			line = *r.Ecosystem + " · " + line
		}
		if r.Version != nil && *r.Version != "" {
			line += " " + *r.Version
		}
		e.ui.Println("    " + line)
	}
}

// discard : destination vide (téléchargement refusé attendu).
type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

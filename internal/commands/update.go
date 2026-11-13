package commands

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"koryhub.dev/kory/internal/i18n"
	"koryhub.dev/kory/internal/install"
	"koryhub.dev/kory/internal/lock"
	"koryhub.dev/kory/internal/semver"
)

// runUpdate met à jour un kory installé (K03.3, F-INT-06) : kory update @auteur/module[@version]
// [--yes]. Jamais sans confirmation ; un fichier modifié depuis l'installation passe toujours par
// l'arbitrage, même avec --yes (il est alors gardé).
func runUpdate(e *env, args []string) int {
	target, assumeYes, ok := parseModuleArgs(e, args, "update.usage")
	if !ok {
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
	installed, found := l.Modules[target.fullName()]
	if !found {
		e.ui.Error(i18n.T("update.not_installed", target.fullName()), i18n.T("update.not_installed_action", target.fullName()))
		return ExitUsage
	}

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
	if detail.Version == installed.Version {
		e.ui.Success(i18n.T("update.up_to_date", target.fullName(), installed.Version))
		return ExitOK
	}
	if target.Version == "" && !newer(detail.Version, installed.Version) {
		e.ui.Success(i18n.T("update.up_to_date", target.fullName(), installed.Version))
		return ExitOK
	}

	e.ui.Println("  " + i18n.T("update.header", module.Name, installed.Version, detail.Version))
	printWarnings(e, module, detail)
	printRequirements(e, detail)
	if code, ok := confirm(e, i18n.T("update.question"), assumeYes); !ok {
		return code
	}

	archive, ok := download(e, root, token, target, detail)
	if !ok {
		return ExitError
	}

	dir := installed.Directory
	items, err := install.Plan(root, dir, archive.Files, &installed)
	if err != nil {
		e.ui.Error(i18n.T("pull.directory_invalid", dir), i18n.T("pull.directory_action"))
		return ExitError
	}
	replace := arbitrate(e, root, dir, items)

	if !apply(e, root, "update", l, target.fullName(), detail, dir, items, replace) {
		return ExitError
	}
	removeDropped(e, root, dir, installed, archive.Files)
	notifyOtherUpdates(e, token, l, target.fullName())

	return ExitOK
}

// removeDropped : fichiers retirés par la nouvelle version ; supprimés s'ils sont inchangés depuis
// l'installation, gardés (et signalés) si vous les avez modifiés.
func removeDropped(e *env, root, dir string, previous lock.Entry, files map[string][]byte) {
	for path, entry := range previous.Files {
		if _, still := files[path]; still {
			continue
		}
		target, err := install.Target(root, dir, path)
		if err != nil {
			continue
		}
		current, err := os.ReadFile(target)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err == nil && !entry.Kept && install.Hash(current) == entry.SHA256 {
			os.Remove(target)
			removeEmptyParents(root, dir, filepath.Dir(target))
			continue
		}
		e.ui.Warn(i18n.T("update.dropped_kept", path), "")
	}
}

// removeEmptyParents supprime les dossiers vides jusqu'au dossier d'installation compris.
func removeEmptyParents(root, dir, from string) {
	stop := filepath.Dir(filepath.Join(root, filepath.FromSlash(dir)))
	for current := from; strings.HasPrefix(current, stop) && current != stop && current != root; current = filepath.Dir(current) {
		if os.Remove(current) != nil {
			return
		}
	}
}

// parseModuleArgs : « @auteur/module[@version] » et --yes, pour update et remove.
func parseModuleArgs(e *env, args []string, usage string) (spec, bool, bool) {
	var target spec
	parsed, assumeYes := false, false
	for _, arg := range args {
		switch {
		case arg == "--yes" || arg == "-y":
			assumeYes = true
		case !strings.HasPrefix(arg, "-") && !parsed:
			target, parsed = parseSpec(arg)
			if !parsed {
				e.ui.Error(i18n.T("pull.invalid_name", arg), i18n.T(usage))
				return spec{}, false, false
			}
		default:
			e.ui.Error(i18n.T(usage), "")
			return spec{}, false, false
		}
	}
	if !parsed {
		e.ui.Error(i18n.T(usage), "")
		return spec{}, false, false
	}

	return target, assumeYes, true
}

func newer(candidate, current string) bool {
	a, errA := semver.Parse(candidate)
	b, errB := semver.Parse(current)

	return errA == nil && errB == nil && a.Compare(b) > 0
}

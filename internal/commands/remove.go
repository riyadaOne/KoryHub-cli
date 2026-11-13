package commands

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"koryhub.dev/kory/internal/i18n"
	"koryhub.dev/kory/internal/install"
	"koryhub.dev/kory/internal/lock"
)

// runRemove retire un kory du projet (K03.4, F-INT-10) : kory remove @auteur/module [--yes].
// Le registre n'est pas modifié. Les fichiers modifiés depuis l'installation sont gardés, sauf
// réponse « oui » à une seconde question (jamais avec --yes seul).
func runRemove(e *env, args []string) int {
	target, assumeYes, ok := parseModuleArgs(e, args, "remove.usage")
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
	entry, found := l.Modules[target.fullName()]
	if !found {
		e.ui.Error(i18n.T("update.not_installed", target.fullName()), i18n.T("remove.not_installed_action"))
		return ExitUsage
	}

	var unchanged, modified []string
	for path, file := range entry.Files {
		full, err := install.Target(root, entry.Directory, path)
		if err != nil {
			continue
		}
		current, err := os.ReadFile(full)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err == nil && !file.Kept && install.Hash(current) == file.SHA256 {
			unchanged = append(unchanged, path)
		} else {
			modified = append(modified, path)
		}
	}
	sort.Strings(unchanged)
	sort.Strings(modified)

	e.ui.Println("  " + i18n.T("remove.files", filepath.FromSlash(entry.Directory), len(unchanged)))
	if len(modified) > 0 {
		e.ui.Warn(i18n.T("remove.modified"), "")
		for _, path := range modified {
			e.ui.Println("    " + path)
		}
	}
	if code, ok := confirm(e, i18n.T("remove.question", target.fullName()), assumeYes); !ok {
		return code
	}

	toDelete := unchanged
	kept := modified
	if len(modified) > 0 {
		if all, _ := e.ui.Confirm(i18n.T("remove.question_modified"), false); all {
			toDelete = append(toDelete, modified...)
			kept = nil
		}
	}

	for _, path := range toDelete {
		full, err := install.Target(root, entry.Directory, path)
		if err != nil {
			continue
		}
		if err := os.Remove(full); err != nil && !errors.Is(err, fs.ErrNotExist) {
			e.ui.Error(i18n.T("pull.write_failed", err.Error()), "")
			return ExitError
		}
		removeEmptyParents(root, entry.Directory, filepath.Dir(full))
	}

	delete(l.Modules, target.fullName())
	if err := l.Save(root); err != nil {
		e.ui.Error(i18n.T("pull.write_failed", err.Error()), "")
		return ExitError
	}
	lock.Log(root, "remove", target.fullName(), entry.Version, entry.Directory)

	e.ui.Success(i18n.T("remove.deleted", len(toDelete)))
	for _, path := range kept {
		e.ui.Println("  " + i18n.T("remove.kept", path))
	}
	e.ui.Success(i18n.T("remove.lock_updated"))

	return ExitOK
}

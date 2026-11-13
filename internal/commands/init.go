package commands

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"koryhub.dev/kory/internal/i18n"
	"koryhub.dev/kory/internal/project"
)

// runInit crée un kory dans le projet (K02.1, F-REG-25) :
// kory init [nom] [--add <chemin>]... [--license <SPDX>].
func runInit(e *env, args []string) int {
	opts := project.InitOptions{License: "MIT"}

	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--add" && i+1 < len(args):
			opts.Add = append(opts.Add, args[i+1])
			i++
		case arg == "--license" && i+1 < len(args):
			opts.License = args[i+1]
			i++
		case !strings.HasPrefix(arg, "-") && opts.Name == "":
			opts.Name = arg
		default:
			e.ui.Error(i18n.T("init.usage"), "")

			return ExitUsage
		}
	}

	root, err := os.Getwd()
	if err != nil {
		e.ui.Error(err.Error(), "")

		return ExitError
	}

	opts.Readme = i18n.T("init.readme_template", displayName(opts.Name))
	k, err := project.Init(root, opts)
	switch {
	case errors.Is(err, project.ErrExists):
		e.ui.Error(i18n.T("init.exists"), i18n.T("init.exists_action"))

		return ExitUsage
	case errors.Is(err, project.ErrInvalidName):
		e.ui.Error(i18n.T("init.invalid_name"), i18n.T("init.name_hint"))

		return ExitUsage
	case err != nil:
		e.ui.Error(i18n.T("init.failed", err.Error()), "")

		return ExitError
	}

	if handle := currentHandle(e); handle != "" {
		e.ui.Println("  " + i18n.T("init.published_as", "@"+handle+"/"+k.Manifest.Name))
	}
	if k.Manifest.Language != "" {
		e.ui.Println("  " + i18n.T("init.language", k.Manifest.Language))
	}
	e.ui.Success(i18n.T("init.manifest_created", relative(root, k.ManifestPath), len(k.Manifest.Files)))
	e.ui.Success(i18n.T("init.readme_created", relative(root, k.ReadmePath)))
	e.ui.Println("  " + i18n.T("init.next", pushCommand(opts.Name)))

	return ExitOK
}

func displayName(name string) string {
	if name == "" {
		return "kory"
	}

	return name
}

func pushCommand(name string) string {
	if name == "" {
		return "kory push --dry-run"
	}

	return fmt.Sprintf("kory push %s --dry-run", name)
}

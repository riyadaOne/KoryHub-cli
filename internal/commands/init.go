package commands

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"koryhub.dev/kory/internal/graph"
	"koryhub.dev/kory/internal/i18n"
	"koryhub.dev/kory/internal/project"
)

// runInit crée un kory dans le projet (K02.1, K02.9, F-REG-25, F-REG-26) :
// kory init [nom] [--add <chemin>]... [--from <chemin>] [--license <SPDX>] [--yes].
func runInit(e *env, args []string) int {
	opts := project.InitOptions{License: "MIT"}
	from := ""
	assumeYes := false

	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--add" && i+1 < len(args):
			opts.Add = append(opts.Add, args[i+1])
			i++
		case arg == "--from" && i+1 < len(args):
			from = args[i+1]
			i++
		case arg == "--license" && i+1 < len(args):
			opts.License = args[i+1]
			i++
		case arg == "--yes" || arg == "-y":
			assumeYes = true
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

	if from != "" {
		if code, ok := prepareExtraction(e, root, from, &opts, assumeYes); !ok {
			return code
		}
	}

	opts.Readme = i18n.T("init.readme_template", displayName(opts.Name))
	k, err := project.Init(root, opts)
	if err == nil {
		// Fichier déjà dans un autre kory du projet : permis, signalé (K02.9).
		for _, s := range project.SharedFiles(root, k.Manifest.Files, k.Manifest.Name) {
			e.ui.Warn(i18n.T("init.shared", s.Path, s.Kory), "")
		}
	}
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

	// Architecture détectée : écrite seulement après confirmation (T-41).
	if from != "" {
		result := graph.Analyze(project.ReadFiles(root, k.Manifest.Files))
		if len(result.Components) > 0 {
			printArchitecture(e, result)
			if code, ok := confirm(e, i18n.T("graph.write_question"), assumeYes); !ok {
				return code
			}
			if err := writeArchitecture(k, result.Architecture); err != nil {
				e.ui.Error(i18n.T("init.failed", err.Error()), "")

				return ExitError
			}
			e.ui.Success(i18n.T("graph.written", relative(root, k.ManifestPath)))
		}
	}

	e.ui.Println("  " + i18n.T("init.next", pushCommand(opts.Name)))

	return ExitOK
}

// prepareExtraction : fichiers du chemin de départ, fichiers reliés hors du chemin (proposés avec
// leur raison, ajoutés après confirmation), prérequis du code extérieur (F-REG-26, F-REG-27).
func prepareExtraction(e *env, root, from string, opts *project.InitOptions, assumeYes bool) (int, bool) {
	if err := project.EnsureIgnoreFile(root); err != nil {
		e.ui.Error(i18n.T("init.failed", err.Error()), "")

		return ExitError, false
	}
	ignore, err := project.LoadIgnore(root)
	if err != nil {
		e.ui.Error(i18n.T("init.failed", err.Error()), "")

		return ExitError, false
	}

	start, related, err := project.Extract(root, from, ignore)
	if err != nil || len(start) == 0 {
		e.ui.Error(i18n.T("init.from_missing", from), i18n.T("init.from_action"))

		return ExitUsage, false
	}
	e.ui.Success(i18n.T("init.from_found", len(start), from))

	files := append([]string{}, start...)
	if len(related) > 0 {
		e.ui.Warn(i18n.T("init.related"), "")
		width := 0
		for _, r := range related {
			width = max(width, len(r.Path))
		}
		for _, r := range related {
			e.ui.Println(fmt.Sprintf("    %-*s  (%s)", width, r.Path, i18n.T("init.related_reason", r.From, r.Name)))
		}
		add, err := e.ui.Confirm(i18n.T("init.related_question"), assumeYes)
		if err != nil {
			e.ui.Error(i18n.T("confirm.non_interactive"), i18n.T("confirm.non_interactive_do"))

			return ExitUsage, false
		}
		if add {
			for _, r := range related {
				files = append(files, r.Path)
			}
		}
	}

	opts.Files = files
	opts.Requires = project.DetectRequirements(root, files)
	for _, r := range opts.Requires {
		label := r.Name
		if r.Version != "" {
			label += " " + r.Version
		}
		e.ui.Warn(i18n.T("init.requirement", r.Ecosystem, label), "")
	}

	return ExitOK, true
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

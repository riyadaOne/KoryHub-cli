package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"koryhub.dev/kory/internal/graph"
	"koryhub.dev/kory/internal/i18n"
	"koryhub.dev/kory/internal/project"
)

// runGraph détecte l'architecture d'un kory et l'écrit dans kory.json après confirmation
// (F-REG-23, T-41) : kory graph [nom] [--yes].
func runGraph(e *env, args []string) int {
	var name string
	assumeYes := false
	for _, arg := range args {
		switch {
		case arg == "--yes" || arg == "-y":
			assumeYes = true
		case !strings.HasPrefix(arg, "-") && name == "":
			name = arg
		default:
			e.ui.Error(i18n.T("graph.usage"), "")

			return ExitUsage
		}
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
	k, ok := project.Find(koryList, name)
	if !ok {
		return reportMissingKory(e, koryList, name, "graph.choose_action")
	}

	result := graph.Analyze(project.ReadFiles(root, k.Manifest.Files))
	printArchitecture(e, result)

	if len(result.Components) == 0 {
		return ExitOK
	}

	if code, ok := confirm(e, i18n.T("graph.write_question"), assumeYes); !ok {
		return code
	}
	if err := writeArchitecture(k, result.Architecture); err != nil {
		e.ui.Error(i18n.T("init.failed", err.Error()), "")

		return ExitError
	}
	e.ui.Success(i18n.T("graph.written", relative(root, k.ManifestPath)))

	return ExitOK
}

func writeArchitecture(k *project.Kory, a graph.Architecture) error {
	if a.Links == nil {
		a.Links = []graph.Link{}
	}
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	k.Manifest.Architecture = data

	return k.Save()
}

// printArchitecture : tableau des composants, liste des liens, langages non reconnus.
func printArchitecture(e *env, r graph.Result) {
	e.ui.Println("  " + i18n.T("graph.summary", len(r.Components), len(r.Links)))
	if len(r.Components) > 0 {
		rows := [][]string{{i18n.T("graph.col_kind"), i18n.T("graph.col_component"), i18n.T("graph.col_file")}}
		for _, c := range r.Components {
			rows = append(rows, []string{i18n.T("graph.kind." + c.Kind), c.ID, c.File})
		}
		printTable(e, rows)
	}
	for _, l := range r.Links {
		e.ui.Println(fmt.Sprintf("    %s → %s  (%s)", l.From, l.To, i18n.T("graph.link."+l.Kind)))
	}
	for _, f := range r.Unrecognized {
		e.ui.Warn(i18n.T("graph.unrecognized", f), "")
	}
}

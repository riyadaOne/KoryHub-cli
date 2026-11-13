package commands

import (
	"fmt"

	"koryhub.dev/kory/internal/i18n"
)

// runHelp liste les commandes disponibles.
func runHelp(e *env, _ []string) int {
	e.ui.Println(i18n.T("help.usage"))
	e.ui.Println("")
	e.ui.Println(i18n.T("help.commands"))

	for _, cmd := range commandList() {
		e.ui.Println(fmt.Sprintf("  %-10s %s", cmd.name, i18n.T(cmd.helpKey)))
	}

	return ExitOK
}

// Package commands contient les commandes de kory, une par fichier (TRD §4.2).
package commands

import (
	"context"
	"errors"
	"io"
	"time"

	"koryhub.dev/kory/internal/api"
	"koryhub.dev/kory/internal/config"
	"koryhub.dev/kory/internal/i18n"
	"koryhub.dev/kory/internal/ui"
)

// Codes de sortie (UXUI §7) : 0 succès, 1 erreur, 2 mauvaise utilisation.
const (
	ExitOK    = 0
	ExitError = 1
	ExitUsage = 2
)

// env regroupe ce dont une commande a besoin.
type env struct {
	ui     *ui.UI
	config *config.Config
}

func (e *env) client() *api.Client {
	return api.NewClient(e.config.EffectiveAPIURL(), string(i18n.Current()))
}

type command struct {
	name    string
	helpKey string
	run     func(e *env, args []string) int
}

func commandList() []command {
	return []command{
		{"init", "help.cmd.init", runInit},
		{"list", "help.cmd.list", runList},
		{"push", "help.cmd.push", runPush},
		{"graph", "help.cmd.graph", runGraph},
		{"pull", "help.cmd.pull", runPull},
		{"update", "help.cmd.update", runUpdate},
		{"remove", "help.cmd.remove", runRemove},
		{"audit", "help.cmd.audit", runAudit},
		{"search", "help.cmd.search", runSearch},
		{"info", "help.cmd.info", runInfo},
		{"login", "help.cmd.login", runLogin},
		{"logout", "help.cmd.logout", runLogout},
		{"whoami", "help.cmd.whoami", runWhoami},
		{"version", "help.cmd.version", runVersion},
		{"config", "help.cmd.config", runConfig},
		{"help", "help.cmd.help", runHelp},
	}
}

// Run exécute kory avec ses arguments et renvoie le code de sortie.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	out := ui.New(stdin, stdout, stderr)

	cfg, err := config.Load()
	if err != nil {
		i18n.Use(i18n.Detect(""))
		out.Error(err.Error(), "")

		return ExitError
	}
	i18n.Use(i18n.Detect(cfg.Lang))

	e := &env{ui: out, config: cfg}

	if len(args) == 0 {
		return runHelp(e, nil)
	}

	name := args[0]
	switch name {
	case "--version", "-v":
		name = "version"
	case "--help", "-h":
		name = "help"
	}

	for _, cmd := range commandList() {
		if cmd.name == name {
			return cmd.run(e, args[1:])
		}
	}

	out.Error(i18n.T("error.unknown_command", name), i18n.T("error.unknown_command_hint"))

	return ExitUsage
}

// reportAPIError affiche une erreur de l'API (message traduit par le serveur) et son action.
func reportAPIError(e *env, err error) int {
	var apiErr *api.Error

	switch {
	case errors.As(err, &apiErr) && apiErr.Code == api.CodeUpgradeRequired:
		e.ui.Error(apiErr.Message, i18n.T("api.upgrade_action"))
	case errors.As(err, &apiErr) && apiErr.Code == api.CodeTokenInvalid:
		e.ui.Error(apiErr.Message, i18n.T("auth.login_action"))
	case errors.As(err, &apiErr):
		e.ui.Error(apiErr.Message, "")
	case errors.Is(err, api.ErrUnreachable):
		e.ui.Error(i18n.T("api.unreachable", e.config.EffectiveAPIURL()), i18n.T("api.unreachable_action"))
	default:
		e.ui.Error(i18n.T("api.invalid_response"), "")
	}

	return ExitError
}

// requestContext limite la durée d'un appel à l'API.
func requestContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 20*time.Second)
}

package commands

import (
	"koryhub.dev/kory/internal/i18n"
)

// runWhoami affiche le compte relié au terminal et l'expiration de son jeton.
func runWhoami(e *env, args []string) int {
	if len(args) > 0 {
		e.ui.Error(i18n.T("error.unknown_command", "whoami "+args[0]), i18n.T("error.unknown_command_hint"))

		return ExitUsage
	}

	creds, ok := loadCredentials(e)
	if !ok {
		return ExitError
	}
	if creds == nil {
		e.ui.Error(i18n.T("auth.not_logged_in"), i18n.T("auth.login_action"))

		return ExitError
	}

	ctx, cancel := requestContext()
	defer cancel()

	user, err := e.client().CurrentUser(ctx, creds.Token)
	if err != nil {
		return reportAPIError(e, err)
	}

	e.ui.Println(formatHandle(user.Handle))
	printDetails(e, [][2]string{
		{i18n.T("report.server"), e.config.EffectiveAPIURL()},
		{i18n.T("report.token"), user.TokenName},
		{i18n.T("report.expires"), expiryText(user.TokenExpiresAt)},
	})

	return ExitOK
}

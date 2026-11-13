package commands

import (
	"errors"

	"koryhub.dev/kory/internal/api"
	"koryhub.dev/kory/internal/credentials"
	"koryhub.dev/kory/internal/i18n"
)

// runLogout révoque le jeton du terminal sur le serveur puis l'efface du poste (F-AUT-08) ;
// serveur injoignable : le jeton est effacé quand même, avec l'adresse pour le révoquer.
func runLogout(e *env, args []string) int {
	if len(args) > 0 {
		e.ui.Error(i18n.T("error.unknown_command", "logout "+args[0]), i18n.T("error.unknown_command_hint"))

		return ExitUsage
	}

	creds, ok := loadCredentials(e)
	if !ok {
		return ExitError
	}
	if creds == nil {
		e.ui.Println(i18n.T("logout.not_logged_in"))

		return ExitOK
	}

	ctx, cancel := requestContext()
	defer cancel()
	revokeErr := e.client().RevokeToken(ctx, creds.Token)

	if err := credentials.Delete(e.config.EffectiveCredentialFile()); err != nil {
		e.ui.Error(err.Error(), "")

		return ExitError
	}

	var apiErr *api.Error
	switch {
	case revokeErr == nil, errors.As(revokeErr, &apiErr) && apiErr.Code == api.CodeTokenInvalid:
		e.ui.Success(i18n.T("logout.done"))
	case errors.As(revokeErr, &apiErr):
		e.ui.Warn(apiErr.Message, i18n.T("logout.revoke_on_site", siteURL(e.config.EffectiveAPIURL())))
	default:
		e.ui.Warn(i18n.T("logout.offline"), i18n.T("logout.revoke_on_site", siteURL(e.config.EffectiveAPIURL())))
	}

	return ExitOK
}

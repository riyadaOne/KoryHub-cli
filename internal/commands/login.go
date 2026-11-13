package commands

import (
	"context"
	"errors"
	"time"

	"koryhub.dev/kory/internal/api"
	"koryhub.dev/kory/internal/browser"
	"koryhub.dev/kory/internal/credentials"
	"koryhub.dev/kory/internal/i18n"
	"koryhub.dev/kory/internal/ui"
)

// wait patiente entre deux interrogations de l'API ; remplacé dans les tests.
var wait = time.Sleep

// runLogin relie le terminal à un compte (F-AUT-06, F-AUT-07, F-AUT-13) :
// kory login (code validé dans le navigateur) ou kory login --with-token (entrée standard).
func runLogin(e *env, args []string) int {
	withToken, assumeYes := false, false
	for _, arg := range args {
		switch arg {
		case "--with-token":
			withToken = true
		case "--yes", "-y":
			assumeYes = true
		default:
			e.ui.Error(i18n.T("login.usage"), "")

			return ExitUsage
		}
	}

	// Un fichier de jeton illisible est simplement remplacé par le nouveau.
	previous, _ := credentials.Load(e.config.EffectiveCredentialFile(), e.config.EffectiveAPIURL())

	// Déjà relié : on demande avant de remplacer (« Non » par défaut), sauf --with-token,
	// qui est déjà une demande explicite.
	if previous != nil && !withToken {
		replace, err := e.ui.Confirm(i18n.T("login.replace"), assumeYes)
		if errors.Is(err, ui.ErrNonInteractive) {
			e.ui.Error(i18n.T("confirm.non_interactive"), i18n.T("confirm.non_interactive_do"))

			return ExitUsage
		}
		if !replace {
			e.ui.Println(i18n.T("login.kept"))

			return ExitOK
		}
	}

	var code int
	if withToken {
		code = loginWithToken(e)
	} else {
		code = loginWithDeviceCode(e)
	}

	// L'ancien jeton de ce terminal est révoqué une fois le nouveau enregistré.
	if code == ExitOK && previous != nil {
		ctx, cancel := requestContext()
		defer cancel()
		_ = e.client().RevokeToken(ctx, previous.Token)
	}

	return code
}

func loginWithToken(e *env) int {
	token, piped := e.ui.ReadPiped()
	if !piped || token == "" {
		e.ui.Error(i18n.T("login.no_token"), i18n.T("login.no_token_action"))

		return ExitUsage
	}

	ctx, cancel := requestContext()
	defer cancel()

	user, err := e.client().CurrentUser(ctx, token)
	if err != nil {
		return reportAPIError(e, err)
	}

	return saveAndReport(e, token, user.Handle, user.TokenName, user.TokenExpiresAt)
}

func loginWithDeviceCode(e *env) int {
	ctx, cancel := requestContext()
	device, err := e.client().RequestDeviceCode(ctx)
	cancel()
	if err != nil {
		return reportAPIError(e, err)
	}

	e.ui.Println(i18n.T("login.open_and_enter", device.VerificationURI))
	e.ui.Println("")
	e.ui.Println("    " + device.UserCode)
	e.ui.Println("")
	if browser.Open(device.VerificationURIComplete) == nil {
		e.ui.Println(i18n.T("login.browser_opened"))
	} else {
		e.ui.Println(i18n.T("login.open_yourself"))
	}

	interval := time.Duration(max(device.Interval, 1)) * time.Second
	deadline := now().Add(time.Duration(device.ExpiresIn) * time.Second)
	warned := false

	for {
		wait(interval)
		if now().After(deadline) {
			e.ui.Error(i18n.T("login.expired"), i18n.T("login.retry"))

			return ExitError
		}

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		issued, err := e.client().PollDeviceToken(ctx, device.DeviceCode)
		cancel()

		var apiErr *api.Error
		switch {
		case err == nil:
			return saveAndReport(e, issued.Token, issued.Handle, issued.TokenName, issued.ExpiresAt)
		case errors.As(err, &apiErr) && apiErr.Code == api.CodeAuthorizationPending:
			continue
		case errors.As(err, &apiErr) && apiErr.Code == api.CodeSlowDown:
			interval += 5 * time.Second
		case errors.As(err, &apiErr) && apiErr.Code == api.CodeExpiredToken:
			e.ui.Error(i18n.T("login.expired"), i18n.T("login.retry"))

			return ExitError
		case errors.As(err, &apiErr) && apiErr.Code == api.CodeAccessDenied:
			e.ui.Error(i18n.T("login.denied"), i18n.T("login.retry"))

			return ExitError
		case errors.Is(err, api.ErrUnreachable):
			// Réseau instable : l'attente continue jusqu'à l'expiration du code.
			if !warned {
				e.ui.Warn(i18n.T("login.still_waiting"), "")
				warned = true
			}
		default:
			return reportAPIError(e, err)
		}
	}
}

func saveAndReport(e *env, token, handle, tokenName string, expiresAt *string) int {
	path := e.config.EffectiveCredentialFile()

	if err := credentials.Save(path, credentials.Credentials{Token: token, APIURL: e.config.EffectiveAPIURL()}); err != nil {
		e.ui.Error(i18n.T("login.save_failed", err.Error()), "")

		return ExitError
	}

	e.ui.Println("")
	e.ui.Success(i18n.T("login.success", handle))
	printDetails(e, [][2]string{
		{i18n.T("report.token"), tokenName},
		{i18n.T("report.expires"), expiryText(expiresAt)},
		{i18n.T("report.file"), path},
	})

	return ExitOK
}

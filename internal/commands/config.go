package commands

import (
	"errors"

	"koryhub.dev/kory/internal/config"
	"koryhub.dev/kory/internal/i18n"
)

// runConfig lit ou change un réglage : kory config get <réglage>, kory config set <réglage> <valeur>.
func runConfig(e *env, args []string) int {
	switch {
	case len(args) == 2 && args[0] == "get":
		value, known := e.config.Get(args[1])
		if !known {
			e.ui.Error(i18n.T("config.unknown_key", args[1]), i18n.T("config.keys_hint"))

			return ExitUsage
		}
		if value == "" && args[1] == "api-url" {
			value = e.config.EffectiveAPIURL()
		}
		if value == "" {
			e.ui.Println(i18n.T("config.not_set", args[1]))

			return ExitOK
		}
		e.ui.Println(value)

		return ExitOK

	case len(args) == 3 && args[0] == "set":
		if err := e.config.Set(args[1], args[2]); err != nil {
			reportConfigError(e, err, args[1], args[2])

			return ExitUsage
		}
		if err := e.config.Save(); err != nil {
			e.ui.Error(i18n.T("config.save_failed", err.Error()), "")

			return ExitError
		}
		if args[1] == "lang" {
			i18n.Use(i18n.Lang(e.config.Lang))
		}
		e.ui.Success(args[1] + " = " + args[2])

		return ExitOK
	}

	e.ui.Error(i18n.T("config.usage"), i18n.T("config.keys_hint"))

	return ExitUsage
}

func reportConfigError(e *env, err error, key, value string) {
	switch {
	case errors.Is(err, config.ErrInvalidLang):
		e.ui.Error(i18n.T("config.invalid_lang", value), i18n.T("config.langs_hint"))
	case errors.Is(err, config.ErrInvalidURL):
		e.ui.Error(i18n.T("config.invalid_url", value), i18n.T("config.url_hint"))
	case errors.Is(err, config.ErrInvalidPath):
		e.ui.Error(i18n.T("config.invalid_path", value), i18n.T("config.path_hint"))
	default:
		e.ui.Error(i18n.T("config.unknown_key", key), i18n.T("config.keys_hint"))
	}
}

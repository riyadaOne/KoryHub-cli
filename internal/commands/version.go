package commands

import (
	"koryhub.dev/kory/internal/i18n"
	"koryhub.dev/kory/internal/semver"
	"koryhub.dev/kory/internal/version"
)

// runVersion affiche la version de kory, puis interroge /v1/meta : une nouvelle
// version est signalée, jamais installée (P3) ; une version trop ancienne reçoit 426.
func runVersion(e *env, _ []string) int {
	e.ui.Println(i18n.T("version.line", version.Version))

	ctx, cancel := requestContext()
	defer cancel()

	meta, err := e.client().Meta(ctx)
	if err != nil {
		return reportAPIError(e, err)
	}

	current, errCurrent := semver.Parse(version.Version)
	latest, errLatest := semver.Parse(meta.LatestCLIVersion)
	if errCurrent == nil && errLatest == nil && current.Compare(latest) < 0 {
		e.ui.Warn(i18n.T("version.update_available", latest.String()), i18n.T("version.update_action"))
	}

	return ExitOK
}

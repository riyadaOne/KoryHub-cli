package commands

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"koryhub.dev/kory/internal/api"
	"koryhub.dev/kory/internal/archive"
	"koryhub.dev/kory/internal/i18n"
	"koryhub.dev/kory/internal/project"
	"koryhub.dev/kory/internal/ui"
)

// runPush publie un kory (K02.4, F-REG-04, F-REG-12, F-REG-32, F-REG-33) :
// kory push [nom] [--dry-run] [--yes] [--public|--private].
func runPush(e *env, args []string) int {
	var name, visibility string
	dryRun, assumeYes := false, false

	for _, arg := range args {
		switch {
		case arg == "--dry-run":
			dryRun = true
		case arg == "--yes" || arg == "-y":
			assumeYes = true
		case arg == "--public":
			visibility = "public"
		case arg == "--private":
			visibility = "private"
		case !strings.HasPrefix(arg, "-") && name == "":
			name = arg
		default:
			e.ui.Error(i18n.T("push.usage"), "")

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
		return reportMissingKory(e, koryList, name)
	}

	// Compte relié : nécessaire pour publier ; pour --dry-run, seulement pour afficher le nom complet.
	creds, loaded := loadCredentials(e)
	if !loaded {
		return ExitError
	}
	var user *api.User
	if creds != nil {
		ctx, cancel := requestContext()
		user, err = e.client().CurrentUser(ctx, creds.Token)
		cancel()
		if err != nil && !dryRun {
			return reportAPIError(e, err)
		}
	}
	if user == nil && !dryRun {
		e.ui.Error(i18n.T("auth.not_logged_in"), i18n.T("auth.login_action"))

		return ExitError
	}

	fullName := k.Manifest.Name
	var module *api.Module
	if user != nil {
		fullName = "@" + user.Handle + "/" + k.Manifest.Name
		ctx, cancel := requestContext()
		module, err = e.client().GetModule(ctx, creds.Token, user.Handle, k.Manifest.Name)
		cancel()
		if err != nil && !dryRun {
			return reportAPIError(e, err)
		}
	}

	result, err := archive.Build(root, k, fullName)
	if err != nil {
		return reportArchiveProblem(e, err)
	}

	printSummary(e, k, fullName, module, user != nil, result)

	if dryRun {
		e.ui.Println("  " + i18n.T("push.dry_run_done"))

		return ExitOK
	}

	request := api.PublishRequest{Archive: result.Data, SHA256: result.SHA256}

	if !user.PublicationTermsAccepted {
		e.ui.Println("")
		for _, line := range strings.Split(i18n.T("push.terms"), "\n") {
			e.ui.Println("  " + line)
		}
		if code, ok := confirm(e, i18n.T("push.terms_question"), assumeYes); !ok {
			return code
		}
		request.RightsCertified, request.NoticeAccepted = true, true
	}

	if module == nil {
		request.Visibility = visibility
		if request.Visibility == "" {
			request.Visibility = "private"
			if e.ui.Interactive() {
				public, _ := e.ui.Confirm(i18n.T("push.make_public"), false)
				if public {
					request.Visibility = "public"
				}
			}
		}
	}

	if code, ok := confirm(e, i18n.T("push.confirm"), assumeYes); !ok {
		return code
	}

	request.Progress = func(sent, total int64) { e.ui.Progress(i18n.T("push.sending"), sent, total) }
	ctx, cancel := requestContext()
	defer cancel()
	published, err := e.client().Publish(ctx, creds.Token, user.Handle, k.Manifest.Name, request)
	if err != nil {
		return reportAPIError(e, err)
	}

	e.ui.Success(i18n.T("push.published", published.Name, published.Version))
	printDetails(e, [][2]string{
		{i18n.T("push.checksum"), published.SHA256},
		{i18n.T("push.address"), published.URL},
	})

	return ExitOK
}

// confirm pose une question « Non » par défaut ; renvoie (code de sortie, continuer).
func confirm(e *env, question string, assumeYes bool) (int, bool) {
	ok, err := e.ui.Confirm(question, assumeYes)
	if errors.Is(err, ui.ErrNonInteractive) {
		e.ui.Error(i18n.T("confirm.non_interactive"), i18n.T("confirm.non_interactive_do"))

		return ExitUsage, false
	}
	if !ok {
		e.ui.Println("  " + i18n.T("push.cancelled"))

		return ExitOK, false
	}

	return ExitOK, true
}

func printSummary(e *env, k *project.Kory, fullName string, module *api.Module, loggedIn bool, result *archive.Result) {
	status := ""
	switch {
	case module != nil && module.LatestVersion != nil:
		status = " · " + i18n.T("push.latest", *module.LatestVersion)
	case loggedIn && module == nil:
		status = " · " + i18n.T("push.new_module")
	}
	e.ui.Println("  " + fullName + " " + k.Manifest.Version + status)

	shown := result.Entries
	if len(shown) > 5 {
		shown = shown[:5]
	}
	width := 0
	for _, entry := range shown {
		width = max(width, len(entry.Path))
	}
	for _, entry := range shown {
		e.ui.Println(fmt.Sprintf("    %-*s  %s", width, entry.Path, kilobytes(entry.Size)))
	}
	if more := len(result.Entries) - len(shown); more > 0 {
		e.ui.Println("    " + i18n.T("push.more_files", more))
	}
	e.ui.Println("  " + i18n.T("push.total", len(result.Entries), kilobytes(int64(len(result.Data)))))
	e.ui.Println("  " + i18n.T("push.checksum") + "  " + result.SHA256)

	if len(k.Manifest.Architecture) == 0 {
		e.ui.Warn(i18n.T("push.no_architecture"), "")
	}
}

func kilobytes(n int64) string {
	return i18n.T("push.kb", max(1, (n+1023)/1024))
}

func reportArchiveProblem(e *env, err error) int {
	var problem *archive.Problem
	if !errors.As(err, &problem) {
		e.ui.Error(err.Error(), "")

		return ExitError
	}

	e.ui.Error(i18n.T("archive."+problem.Code, problem.Path), i18n.T("archive."+problem.Code+"_action"))

	return ExitError
}

func reportMissingKory(e *env, koryList []project.Kory, name string) int {
	if len(koryList) == 0 {
		e.ui.Error(i18n.T("push.no_kory"), i18n.T("push.no_kory_action"))

		return ExitUsage
	}

	names := make([]string, 0, len(koryList))
	for _, k := range koryList {
		names = append(names, k.Manifest.Name)
	}
	if name == "" {
		e.ui.Error(i18n.T("push.choose"), i18n.T("push.choose_action", strings.Join(names, ", ")))
	} else {
		e.ui.Error(i18n.T("push.unknown_kory", name), i18n.T("push.choose_action", strings.Join(names, ", ")))
	}

	return ExitUsage
}

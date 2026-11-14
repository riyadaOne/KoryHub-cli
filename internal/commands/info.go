package commands

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"koryhub.dev/kory/internal/api"
	"koryhub.dev/kory/internal/i18n"
)

// Limites d'affichage de kory info : le détail complet est sur la fiche du site.
const (
	infoMaxVersions = 5
	infoMaxLinks    = 10
	infoMaxFiles    = 30
)

// runInfo montre un kory avant toute installation (K04.2, F-RCH-06, F-RCH-18) :
// kory info @auteur/module[@version].
func runInfo(e *env, args []string) int {
	if len(args) != 1 {
		e.ui.Error(i18n.T("info.usage"), "")
		return ExitUsage
	}
	target, ok := parseSpec(args[0])
	if !ok {
		e.ui.Error(i18n.T("pull.invalid_name", args[0]), i18n.T("info.usage"))
		return ExitUsage
	}

	creds, loaded := loadCredentials(e)
	if !loaded {
		return ExitError
	}
	token := ""
	if creds != nil {
		token = creds.Token
	}

	ctx, cancel := requestContext()
	defer cancel()
	module, err := e.client().GetModule(ctx, token, target.Author, target.Name)
	if err != nil {
		return reportAPIError(e, err)
	}
	if module == nil {
		e.ui.Error(i18n.T("pull.not_found", target.fullName()), i18n.T("pull.not_found_action"))
		return ExitError
	}

	number := target.Version
	if number == "" && module.LatestVersion != nil {
		number = *module.LatestVersion
	}
	if number == "" && len(module.Versions) > 0 {
		number = module.Versions[0].Version
	}
	var detail *api.VersionDetail
	if number != "" {
		detail, err = e.client().GetVersion(ctx, token, target.Author, target.Name, number)
		var apiErr *api.Error
		if errors.As(err, &apiErr) && apiErr.Status == 404 {
			e.ui.Error(i18n.T("pull.version_not_found", target.fullName(), number), i18n.T("pull.not_found_action"))
			return ExitError
		}
		if err != nil {
			return reportAPIError(e, err)
		}
	}

	printInfoHeader(e, module, detail)
	printInfoVersions(e, module)
	if detail != nil {
		printInfoRequirements(e, detail)
		printInfoArchitecture(e, detail)
		printInfoFiles(e, detail)
	}
	e.ui.Println("")
	e.ui.Println("  " + i18n.T("info.install", target.fullName()))

	return ExitOK
}

func printInfoHeader(e *env, module *api.Module, detail *api.VersionDetail) {
	title := module.Name
	if detail != nil {
		title += "  " + detail.Version
	}
	e.ui.Println("  " + title)
	if module.Description != nil && *module.Description != "" {
		e.ui.Println("  " + *module.Description)
	}

	author := "@" + module.Author.Handle
	if module.Author.Country != nil && *module.Author.Country != "" {
		author += " · " + *module.Author.Country
	}
	e.ui.Println(fmt.Sprintf("  %-10s %s", i18n.T("info.author"), author))

	var facts []string
	if detail != nil {
		facts = append(facts, detail.License)
	}
	if module.Language != nil {
		facts = append(facts, *module.Language)
	}
	if module.Category != nil {
		facts = append(facts, *module.Category)
	}
	e.ui.Println(fmt.Sprintf("  %-10s %s", i18n.T("info.license"), strings.Join(facts, " · ")))

	rating := i18n.T("info.no_rating")
	if module.RatingAvg != nil {
		rating = i18n.T("info.rating", i18n.Decimal(*module.RatingAvg), module.RatingCount)
	}
	e.ui.Println(fmt.Sprintf("  %-10s %s · ↓ %d", i18n.T("info.rating_label"), rating, module.DownloadCount))

	if module.Certified {
		e.ui.Success(i18n.T("info.certified"))
	} else {
		e.ui.Warn(i18n.T("info.not_certified"), "")
	}
	if module.Status == "archived" {
		e.ui.Warn(i18n.T("pull.archived"), "")
	}
}

func printInfoVersions(e *env, module *api.Module) {
	if len(module.Versions) == 0 {
		return
	}
	e.ui.Println("")
	e.ui.Println("  " + i18n.T("info.versions"))
	for i, v := range module.Versions {
		if i == infoMaxVersions {
			e.ui.Println("    " + i18n.T("info.more", len(module.Versions)-infoMaxVersions))
			break
		}
		date := ""
		if v.PublishedAt != nil && len(*v.PublishedAt) >= 10 {
			date = (*v.PublishedAt)[:10]
		}
		line := fmt.Sprintf("    %-10s %-12s %s", v.Version, i18n.T("info.status."+v.Status), date)
		if i == 0 {
			line += "   " + shortHash(v.SHA256)
		}
		if v.DeprecationMessage != nil && *v.DeprecationMessage != "" {
			line += "   " + *v.DeprecationMessage
		}
		e.ui.Println(strings.TrimRight(line, " "))
	}
}

func printInfoRequirements(e *env, detail *api.VersionDetail) {
	if len(detail.Requires) == 0 {
		return
	}
	e.ui.Println("")
	e.ui.Println("  " + i18n.T("info.requirements"))
	for _, r := range detail.Requires {
		line := r.Name
		if r.Ecosystem != nil && *r.Ecosystem != "" {
			line = *r.Ecosystem + " · " + line
		}
		if r.Version != nil && *r.Version != "" {
			line += " " + *r.Version
		}
		e.ui.Println("    " + line)
	}
}

func printInfoArchitecture(e *env, detail *api.VersionDetail) {
	a := detail.Architecture
	if len(a.Components) == 0 {
		return
	}
	e.ui.Println("")
	e.ui.Println("  " + i18n.T("info.architecture", len(a.Components), len(a.Links)))
	for i, l := range a.Links {
		if i == infoMaxLinks {
			e.ui.Println("    …")
			break
		}
		e.ui.Println(fmt.Sprintf("    %s %s %s", l.From, i18n.T("graph.link."+l.Kind), l.To))
	}
}

// printInfoFiles : arborescence des fichiers (sans kory.json), 30 lignes au plus.
func printInfoFiles(e *env, detail *api.VersionDetail) {
	var paths []string
	for _, f := range detail.Files {
		if f.Path != "kory.json" {
			paths = append(paths, f.Path)
		}
	}
	if len(paths) == 0 {
		return
	}
	sort.Strings(paths)

	e.ui.Println("")
	e.ui.Println("  " + i18n.T("info.files"))
	shown := map[string]bool{}
	lines := 0
	for _, p := range paths {
		parts := strings.Split(p, "/")
		for depth := range parts {
			key := strings.Join(parts[:depth+1], "/")
			if shown[key] {
				continue
			}
			shown[key] = true
			if lines == infoMaxFiles {
				e.ui.Println("    …")
				return
			}
			label := parts[depth]
			if depth < len(parts)-1 {
				label += "/"
			}
			e.ui.Println("    " + strings.Repeat("  ", depth) + label)
			lines++
		}
	}
}

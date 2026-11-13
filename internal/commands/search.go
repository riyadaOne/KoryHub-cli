package commands

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"koryhub.dev/kory/internal/api"
	"koryhub.dev/kory/internal/i18n"
)

// runSearch cherche des kory (K04.1, F-RCH-01, F-RCH-05) :
// kory search <mots> [--category <slug>] [--language <langage>] [--limit <n>].
func runSearch(e *env, args []string) int {
	var query api.SearchQuery
	var words []string
	query.Limit = 20

	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--category" && i+1 < len(args):
			query.Category = args[i+1]
			i++
		case arg == "--language" && i+1 < len(args):
			query.Language = args[i+1]
			i++
		case arg == "--limit" && i+1 < len(args):
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 1 || n > 100 {
				e.ui.Error(i18n.T("search.limit_invalid"), i18n.T("search.usage"))
				return ExitUsage
			}
			query.Limit = n
			i++
		case strings.HasPrefix(arg, "-"):
			e.ui.Error(i18n.T("search.usage"), "")
			return ExitUsage
		default:
			words = append(words, arg)
		}
	}
	query.Words = strings.Join(words, " ")
	if query.Words == "" && query.Category == "" && query.Language == "" {
		e.ui.Error(i18n.T("search.usage"), "")
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
	response, err := e.client().Search(ctx, token, query)
	if err != nil {
		return reportAPIError(e, err)
	}

	if len(response.Results) == 0 {
		e.ui.Println("  " + i18n.T("search.none", query.Words))
		e.ui.Println("  " + i18n.T("search.none_action"))
		return ExitOK
	}

	rows := [][]string{{i18n.T("search.col_kory"), i18n.T("search.col_version"), i18n.T("search.col_rating"), "↓", i18n.T("search.col_description")}}
	for _, r := range response.Results {
		version := "—"
		if r.LatestVersion != nil {
			version = *r.LatestVersion
		}
		if r.Deprecated {
			version += " " + i18n.T("search.deprecated")
		}
		if r.Archived {
			version += " " + i18n.T("search.archived")
		}
		rating := "—"
		if r.RatingAvg != nil {
			rating = fmt.Sprintf("%s (%d)", i18n.Decimal(*r.RatingAvg), r.RatingCount)
		}
		description := ""
		if r.Description != nil {
			description = *r.Description
		}
		rows = append(rows, []string{r.Name, version, rating, strconv.FormatInt(r.DownloadCount, 10), description})
	}
	truncateLastColumn(rows, e.ui.Width())
	printTable(e, rows)
	e.ui.Println("  " + i18n.T("search.footer", response.Total))

	return ExitOK
}

// truncateLastColumn coupe la dernière colonne pour que chaque ligne tienne dans width (0 : rien).
func truncateLastColumn(rows [][]string, width int) {
	if width == 0 || len(rows) == 0 {
		return
	}
	last := len(rows[0]) - 1
	used := 2 // retrait
	for col := 0; col < last; col++ {
		widest := 0
		for _, row := range rows {
			widest = max(widest, utf8.RuneCountInString(row[col]))
		}
		used += widest + 3 // espace entre colonnes de printTable
	}
	room := width - used
	if room < 10 {
		room = 10
	}
	for _, row := range rows {
		if runes := []rune(row[last]); len(runes) > room {
			row[last] = string(runes[:room-1]) + "…"
		}
	}
}

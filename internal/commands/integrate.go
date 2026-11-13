package commands

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"koryhub.dev/kory/internal/api"
	"koryhub.dev/kory/internal/diff"
	"koryhub.dev/kory/internal/i18n"
	"koryhub.dev/kory/internal/install"
	"koryhub.dev/kory/internal/lock"
)

// Parties communes à kory pull et kory update (K03.1 à K03.3, APPFLOW §5.3).

var moduleSpec = regexp.MustCompile(`^@([a-z0-9-]+)/([a-z0-9-]+)(?:@([0-9A-Za-z.+-]+))?$`)

// spec : « @auteur/module[@version] ».
type spec struct {
	Author, Name, Version string
}

func (s spec) fullName() string { return "@" + s.Author + "/" + s.Name }

func parseSpec(arg string) (spec, bool) {
	m := moduleSpec.FindStringSubmatch(arg)
	if m == nil {
		return spec{}, false
	}

	return spec{Author: m[1], Name: m[2], Version: m[3]}, true
}

// downloadAttempts : nouvel essai après une coupure réseau (TRD §17).
const downloadAttempts = 3

var retryPause = func(attempt int) { time.Sleep(time.Duration(attempt) * time.Second) }

// download récupère l'archive dans .kory/tmp, vérifie son empreinte, et la lit ; rien n'est
// écrit dans le projet. Le fichier temporaire est toujours effacé.
func download(e *env, root, token string, s spec, detail *api.VersionDetail) (*install.Archive, bool) {
	tmpDir := filepath.Join(root, ".kory", "tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		e.ui.Error(i18n.T("pull.write_failed", err.Error()), "")
		return nil, false
	}

	label := i18n.T("pull.downloading")
	var data []byte
	for attempt := 1; ; attempt++ {
		tmp, err := os.CreateTemp(tmpDir, "download-*.tar.gz")
		if err != nil {
			e.ui.Error(i18n.T("pull.write_failed", err.Error()), "")
			return nil, false
		}

		hasher := sha256.New()
		err = e.client().Download(context.Background(), token, s.Author, s.Name, detail.Version, io.MultiWriter(tmp, hasher), func(done, total int64) {
			e.ui.Progress(label, done, total)
		})
		tmp.Close()

		if err == nil {
			if hex.EncodeToString(hasher.Sum(nil)) != detail.SHA256 {
				os.Remove(tmp.Name())
				e.ui.Error(i18n.T("pull.checksum_mismatch"), i18n.T("pull.retry_action"))
				return nil, false
			}
			data, err = os.ReadFile(tmp.Name())
			os.Remove(tmp.Name())
			if err != nil {
				e.ui.Error(i18n.T("pull.write_failed", err.Error()), "")
				return nil, false
			}
			break
		}

		os.Remove(tmp.Name())
		if !errors.Is(err, api.ErrUnreachable) {
			reportDownloadError(e, err)
			return nil, false
		}
		if attempt == downloadAttempts {
			e.ui.Error(i18n.T("pull.network_cut"), i18n.T("pull.network_action"))
			return nil, false
		}
		e.ui.Println("  " + i18n.T("pull.retrying", attempt+1, downloadAttempts))
		retryPause(attempt)
	}
	e.ui.Success(i18n.T("pull.checksum_ok", shortHash(detail.SHA256)))

	archive, err := install.Read(data)
	if err != nil {
		e.ui.Error(i18n.T("pull.unsafe_archive", err.Error()), "")
		return nil, false
	}

	// Chaque fichier doit correspondre à l'empreinte enregistrée par le serveur.
	expected := map[string]string{}
	for _, f := range detail.Files {
		expected[f.Path] = f.SHA256
	}
	for path, content := range archive.Files {
		if expected[path] != install.Hash(content) {
			e.ui.Error(i18n.T("pull.checksum_mismatch"), i18n.T("pull.retry_action"))
			return nil, false
		}
	}

	return archive, true
}

// reportDownloadError : limite atteinte (heure de reprise locale), version retirée, autres erreurs.
func reportDownloadError(e *env, err error) {
	var apiErr *api.Error
	if errors.As(err, &apiErr) && (apiErr.Code == api.CodeDownloadLimitGuest || apiErr.Code == api.CodeDownloadLimitUser) {
		at := "?"
		if t, parseErr := time.Parse(time.RFC3339, apiErr.RetryAt); parseErr == nil {
			at = t.Local().Format("15:04")
		}
		action := i18n.T("pull.limit_user_action", at)
		if apiErr.Code == api.CodeDownloadLimitGuest {
			action = i18n.T("pull.limit_guest_action", at)
		}
		e.ui.Error(apiErr.Message, action)

		return
	}
	reportAPIError(e, err)
}

// arbitrate demande, pour chaque fichier existant et différent, de le garder ou le remplacer
// (F-INT-03). Sans terminal, ou avec --yes, les fichiers existants sont gardés : jamais
// d'écrasement sans question.
func arbitrate(e *env, root, dir string, items []install.Item) map[string]bool {
	var choices []install.Item
	for _, item := range items {
		if item.NeedsChoice() {
			choices = append(choices, item)
		}
	}
	replace := map[string]bool{}
	if len(choices) == 0 {
		return replace
	}

	if !e.ui.Interactive() {
		e.ui.Warn(i18n.T("pull.kept_without_terminal", len(choices)), i18n.T("pull.kept_without_terminal_action"))
		return replace
	}

	e.ui.Warn(i18n.T("pull.conflicts", len(choices), dir), "")
	keys := []rune(i18n.T("pull.choice_keys")) // garder, remplacer, différence
	for index, item := range choices {
		reason := i18n.T("pull.state_conflict")
		if item.State == install.Modified {
			reason = i18n.T("pull.state_modified")
		}
		e.ui.Println("")
		e.ui.Println(fmt.Sprintf("  %s  (%s)", item.Path, reason))

		withDiff := true
		decision := false
		for {
			question := i18n.T("pull.choice_question")
			if !withDiff {
				question = i18n.T("pull.choice_question_short")
			}
			answer, ok := e.ui.Prompt(question)
			if !ok {
				break
			}
			key := []rune(strings.ToLower(answer + " "))[0]
			if withDiff && key == keys[2] {
				showDiff(e, root, dir, item)
				withDiff = false
				continue
			}
			decision = key == keys[1]
			break
		}
		replace[item.Path] = decision

		rest := choices[index+1:]
		if len(rest) > 0 {
			same, _ := e.ui.Confirm(i18n.T("pull.same_choice", len(rest)), false)
			if same {
				for _, r := range rest {
					replace[r.Path] = decision
				}
				break
			}
		}
	}

	return replace
}

// showDiff : différence entre le fichier du projet (−) et celui du kory (+), 200 lignes au plus.
func showDiff(e *env, root, dir string, item install.Item) {
	target, err := install.Target(root, dir, item.Path)
	if err != nil {
		return
	}
	current, _ := os.ReadFile(target)

	hunks, err := diff.Compute(diff.Lines(string(current)), diff.Lines(string(item.Data)), 3)
	if err != nil {
		e.ui.Warn(i18n.T("pull.diff_too_long"), "")
		return
	}

	e.ui.DiffLine("@", "--- "+i18n.T("pull.diff_yours"))
	e.ui.DiffLine("@", "+++ "+i18n.T("pull.diff_kory"))
	printed := 0
	total := 0
	for _, h := range hunks {
		total += len(h.Lines) + 1
	}
	for _, h := range hunks {
		if printed >= maxDiffLines {
			break
		}
		e.ui.DiffLine("@", fmt.Sprintf("@@ %s @@", i18n.T("pull.diff_lines", h.FromLine)))
		printed++
		for _, l := range h.Lines {
			if printed >= maxDiffLines {
				break
			}
			e.ui.DiffLine(map[diff.Kind]string{diff.Same: " ", diff.Removed: "-", diff.Added: "+"}[l.Kind], l.Text)
			printed++
		}
	}
	if total > printed {
		e.ui.DiffLine("@", i18n.T("pull.diff_more", total-printed))
	}
}

const maxDiffLines = 200

// apply écrit les fichiers après l'arbitrage puis kory.lock, en dernier (T-26).
func apply(e *env, root string, l *lock.Lock, name string, detail *api.VersionDetail, dir string, items []install.Item, replace map[string]bool) bool {
	result, err := install.Write(root, dir, items, replace)
	if err != nil {
		e.ui.Error(i18n.T("pull.write_failed", err.Error()), "")
		return false
	}

	l.Modules[name] = lock.Entry{
		Version:     detail.Version,
		SHA256:      detail.SHA256,
		Directory:   filepath.ToSlash(dir),
		InstalledAt: lock.Now(),
		Files:       result.Files,
	}
	if err := l.Save(root); err != nil {
		e.ui.Error(i18n.T("pull.write_failed", err.Error()), "")
		return false
	}

	e.ui.Success(i18n.T("pull.written", result.Written+result.Replaced+result.Identical, filepath.FromSlash(dir)))
	if result.Replaced > 0 || result.Kept > 0 {
		e.ui.Println("  " + i18n.T("pull.summary", result.Written, result.Replaced, result.Kept))
	}
	e.ui.Success(i18n.T("pull.lock_updated"))

	return true
}

// printWarnings : avertissements avant l'installation (F-INT-08, F-INT-09, F-INT-13).
func printWarnings(e *env, module *api.Module, detail *api.VersionDetail) {
	if !module.Certified {
		e.ui.Warn(i18n.T("pull.not_certified"), "")
	}
	if detail.Status == "deprecated" {
		message := ""
		if detail.DeprecationMessage != nil {
			message = *detail.DeprecationMessage
		}
		e.ui.Warn(i18n.T("pull.deprecated", detail.Version), message)
	}
	if module.Status == "archived" {
		e.ui.Warn(i18n.T("pull.archived"), "")
	}
	if detail.Prerelease {
		e.ui.Warn(i18n.T("pull.prerelease", detail.Version), "")
	}
}

func shortHash(h string) string {
	if len(h) < 10 {
		return h
	}

	return h[:6] + "…" + h[len(h)-4:]
}

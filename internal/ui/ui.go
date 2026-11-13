// Package ui écrit dans le terminal selon l'UXUI §7 : symboles ✓ ! ✗, couleurs
// seulement dans un terminal et sans NO_COLOR, une ligne de cause et une ligne
// d'action, questions oui/non dont la réponse par défaut est « Non ».
package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"koryhub.dev/kory/internal/i18n"
)

const (
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	red    = "\x1b[31m"
	dim    = "\x1b[2m"
	reset  = "\x1b[0m"
)

// ErrNonInteractive : une confirmation est nécessaire mais aucun terminal n'est attaché.
var ErrNonInteractive = errors.New("confirmation needed without a terminal")

// UI écrit les messages du CLI.
type UI struct {
	in          *bufio.Reader
	out, err    io.Writer
	color       bool
	interactive bool
}

// New prépare la sortie ; les couleurs et les questions dépendent du terminal.
func New(in io.Reader, out, errOut io.Writer) *UI {
	terminal := isTerminal(out)
	if terminal {
		enableVirtualTerminal(out)
	}

	return &UI{
		in:          bufio.NewReader(in),
		out:         out,
		err:         errOut,
		color:       terminal && os.Getenv("NO_COLOR") == "",
		interactive: terminal && isTerminal(in),
	}
}

// Println écrit une ligne simple.
func (u *UI) Println(text string) {
	fmt.Fprintln(u.out, text)
}

// Success écrit « ✓ texte » en vert.
func (u *UI) Success(text string) {
	fmt.Fprintln(u.out, u.paint(green, "✓")+" "+text)
}

// Warn écrit « ! cause » et, si elle est donnée, la ligne d'action.
func (u *UI) Warn(cause, action string) {
	u.twoLines(u.out, u.paint(yellow, "!"), cause, action)
}

// Error écrit « ✗ cause » et, si elle est donnée, la ligne d'action, sur la sortie d'erreur.
func (u *UI) Error(cause, action string) {
	u.twoLines(u.err, u.paint(red, "✗"), cause, action)
}

// Confirm pose une question oui/non ; « Non » par défaut. Sans terminal, rien n'est
// demandé : ErrNonInteractive, sauf si assumeYes (option --yes).
func (u *UI) Confirm(question string, assumeYes bool) (bool, error) {
	if assumeYes {
		return true, nil
	}
	if !u.interactive {
		return false, ErrNonInteractive
	}

	fmt.Fprintf(u.out, "  %s %s ", question, i18n.T("confirm.yes_no"))

	answer, err := u.in.ReadString('\n')
	if err != nil && answer == "" {
		return false, nil
	}

	return i18n.IsYes(strings.ToLower(strings.TrimSpace(answer))), nil
}

func (u *UI) twoLines(w io.Writer, symbol, cause, action string) {
	fmt.Fprintln(w, symbol+" "+cause)
	if action != "" {
		fmt.Fprintln(w, "  "+u.paint(dim, action))
	}
}

func (u *UI) paint(color, text string) string {
	if !u.color {
		return text
	}

	return color + text + reset
}

// isTerminal indique si w ou r est une console (et non un fichier ou un tube).
func isTerminal(stream any) bool {
	file, ok := stream.(*os.File)
	if !ok {
		return false
	}

	info, err := file.Stat()

	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

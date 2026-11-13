package ui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestOutputWithoutTerminalHasNoColour(t *testing.T) {
	var out, errOut bytes.Buffer
	u := New(strings.NewReader(""), &out, &errOut)

	u.Success("Publié")
	u.Warn("Version dépréciée", "Passez en 1.1.0.")
	u.Error("Secret détecté", "Retirez la clé.")

	if strings.Contains(out.String()+errOut.String(), "\x1b[") {
		t.Error("aucune couleur attendue hors d'un terminal")
	}
	if out.String() != "✓ Publié\n! Version dépréciée\n  Passez en 1.1.0.\n" {
		t.Errorf("sortie : %q", out.String())
	}
	if errOut.String() != "✗ Secret détecté\n  Retirez la clé.\n" {
		t.Errorf("erreur : %q", errOut.String())
	}
}

func TestConfirmWithoutTerminalAsksForYes(t *testing.T) {
	u := New(strings.NewReader("y\n"), &bytes.Buffer{}, &bytes.Buffer{})

	if _, err := u.Confirm("Publier ?", false); !errors.Is(err, ErrNonInteractive) {
		t.Errorf("sans terminal, une confirmation doit être refusée : %v", err)
	}

	if ok, err := u.Confirm("Publier ?", true); !ok || err != nil {
		t.Error("--yes doit confirmer sans poser la question")
	}
}

func TestConfirmDefaultsToNo(t *testing.T) {
	for input, want := range map[string]bool{"\n": false, "n\n": false, "y\n": true, "yes\n": true, "": false} {
		u := New(strings.NewReader(input), &bytes.Buffer{}, &bytes.Buffer{})
		u.interactive = true

		if got, _ := u.Confirm("Publier ?", false); got != want {
			t.Errorf("réponse %q : %v ; attendu %v", input, got, want)
		}
	}
}

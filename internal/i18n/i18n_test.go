package i18n

import (
	"slices"
	"testing"
)

func TestEveryLanguageHasTheSameKeysAsEnglish(t *testing.T) {
	english := Keys(English)
	slices.Sort(english)

	for _, lang := range Supported {
		keys := Keys(lang)
		slices.Sort(keys)

		if !slices.Equal(english, keys) {
			t.Errorf("%s : clés différentes de l'anglais", lang)
		}

		for _, key := range keys {
			if messages[lang][key] == "" {
				t.Errorf("%s : %s est vide", lang, key)
			}
		}
	}
}

func TestParseRecognisesSystemLocaleFormats(t *testing.T) {
	cases := map[string]Lang{"fr": French, "fr-FR": French, "fr_FR.UTF-8": French, "ar-TD": Arabic, "pt_BR": Portuguese, "EN": English}

	for code, want := range cases {
		if got, ok := Parse(code); !ok || got != want {
			t.Errorf("Parse(%q) = %s, %v ; attendu %s", code, got, ok, want)
		}
	}

	if _, ok := Parse("de-DE"); ok {
		t.Error("l'allemand ne doit pas être reconnu")
	}
}

func TestDetectPrefersTheConfiguredLanguage(t *testing.T) {
	if got := Detect("pt"); got != Portuguese {
		t.Errorf("Detect(pt) = %s", got)
	}
}

func TestTranslationFallsBackToEnglishThenKey(t *testing.T) {
	Use(French)
	defer Use(English)

	if got := T("version.line", "1.2.3"); got != "kory 1.2.3" {
		t.Errorf("T = %q", got)
	}
	if got := T("cle.inconnue"); got != "cle.inconnue" {
		t.Errorf("clé inconnue : %q", got)
	}
}

func TestYesAnswersDependOnTheLanguage(t *testing.T) {
	Use(French)
	defer Use(English)

	if !IsYes("o") || !IsYes("y") || IsYes("n") || IsYes("") {
		t.Error("réponses oui/non incorrectes en français")
	}
}

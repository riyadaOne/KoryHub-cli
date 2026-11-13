// Package i18n traduit les messages du CLI en anglais, français, arabe et portugais
// (UXUI §7, TRD §18). Les messages de l'API arrivent déjà traduits par le serveur.
package i18n

import (
	"fmt"
	"strconv"
	"strings"
)

// Lang est une langue proposée.
type Lang string

const (
	English    Lang = "en"
	French     Lang = "fr"
	Arabic     Lang = "ar"
	Portuguese Lang = "pt"
)

// Supported liste les langues proposées, l'anglais en premier (langue par défaut).
var Supported = []Lang{English, French, Arabic, Portuguese}

var current = English

// Parse reconnaît une langue à partir d'un code (« fr », « fr-FR », « fr_FR.UTF-8 »).
func Parse(code string) (Lang, bool) {
	code = strings.ToLower(strings.TrimSpace(code))
	if i := strings.IndexAny(code, "-_."); i >= 0 {
		code = code[:i]
	}

	for _, lang := range Supported {
		if string(lang) == code {
			return lang, true
		}
	}

	return English, false
}

// Detect choisit la langue : réglage de kory, sinon langue du système, sinon anglais.
func Detect(configured string) Lang {
	if lang, ok := Parse(configured); ok {
		return lang
	}

	if lang, ok := Parse(systemLocale()); ok {
		return lang
	}

	return English
}

// Use fixe la langue des messages.
func Use(lang Lang) {
	current = lang
}

// Current renvoie la langue des messages.
func Current() Lang {
	return current
}

// T renvoie le message traduit ; l'anglais sert de secours, la clé en dernier recours.
func T(key string, args ...any) string {
	text, ok := messages[current][key]
	if !ok {
		text, ok = messages[English][key]
	}
	if !ok {
		return key
	}
	if len(args) > 0 {
		return fmt.Sprintf(text, args...)
	}

	return text
}

// Keys renvoie les clés d'une langue (pour vérifier qu'aucune ne manque).
func Keys(lang Lang) []string {
	keys := make([]string, 0, len(messages[lang]))
	for key := range messages[lang] {
		keys = append(keys, key)
	}

	return keys
}

// Decimal écrit un nombre à une décimale avec le séparateur de la langue (« 4,6 » en français,
// en arabe et en portugais, « 4.6 » en anglais).
func Decimal(value float64) string {
	text := strconv.FormatFloat(value, 'f', 1, 64)
	if Current() != English {
		text = strings.Replace(text, ".", ",", 1)
	}

	return text
}

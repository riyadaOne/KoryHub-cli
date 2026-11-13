package graph

import (
	"regexp"
	"strings"
)

var (
	phpClass     = regexp.MustCompile(`(?m)^\s*(?:(?:abstract|final|readonly)\s+)*class\s+([A-Za-z_]\w*)(?:\s+extends\s+([\w\\]+))?(?:\s+implements\s+([\w\\,\s]+?))?\s*\{`)
	phpInterface = regexp.MustCompile(`(?m)^\s*interface\s+([A-Za-z_]\w*)(?:\s+extends\s+([\w\\,\s]+?))?\s*\{`)
	phpTrait     = regexp.MustCompile(`(?m)^\s*trait\s+([A-Za-z_]\w*)`)
	phpEnum      = regexp.MustCompile(`(?m)^\s*enum\s+([A-Za-z_]\w*)(?:\s*:\s*\w+)?(?:\s+implements\s+([\w\\,\s]+?))?\s*\{`)
	phpFunction  = regexp.MustCompile(`(?m)^function\s+([A-Za-z_]\w*)\s*\(`)
	phpTraitUse  = regexp.MustCompile(`(?m)^\s{2,}use\s+([\w\\,\s]+);`)

	jsClass     = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:default\s+)?(?:abstract\s+)?class\s+([A-Za-z_$][\w$]*)(?:<[^>{]*>)?(?:\s+extends\s+([\w$.]+)(?:<[^>{]*>)?)?(?:\s+implements\s+([\w$.,<>\s]+?))?\s*\{`)
	tsInterface = regexp.MustCompile(`(?m)^\s*(?:export\s+)?interface\s+([A-Za-z_$][\w$]*)(?:<[^>{]*>)?(?:\s+extends\s+([\w$.,<>\s]+?))?\s*\{`)
	tsEnum      = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:const\s+)?enum\s+([A-Za-z_$][\w$]*)`)
	jsFunction  = regexp.MustCompile(`(?m)^\s*export\s+(?:default\s+)?(?:async\s+)?function\s*\*?\s*([A-Za-z_$][\w$]*)`)
	jsArrow     = regexp.MustCompile(`(?m)^\s*export\s+const\s+([A-Za-z_$][\w$]*)\s*(?::[^=]+)?=\s*(?:async\s+)?(?:\([^)]*\)|[A-Za-z_$][\w$]*)\s*=>`)

	dartClass = regexp.MustCompile(`(?m)^\s*(?:(?:abstract|base|final|sealed|interface)\s+)*class\s+([A-Za-z_]\w*)(?:<[^>{]*>)?(?:\s+extends\s+([\w.]+)(?:<[^>{]*>)?)?(?:\s+with\s+([\w.,<>\s]+?))?(?:\s+implements\s+([\w.,<>\s]+?))?\s*\{`)
	dartMixin = regexp.MustCompile(`(?m)^\s*(?:base\s+)?mixin\s+([A-Za-z_]\w*)`)
	dartEnum  = regexp.MustCompile(`(?m)^\s*enum\s+([A-Za-z_]\w*)`)
)

// parsePHP : classes, interfaces, traits, enums ; fonctions de premier niveau s'il n'y a pas de classe.
func parsePHP(source string) parsed {
	code := clean(source, true)
	var decls []declaration

	for _, m := range phpClass.FindAllStringSubmatch(code, -1) {
		decls = append(decls, declaration{name: m[1], kind: "class", extends: names(m[2]), implements: names(m[3])})
	}
	for _, m := range phpInterface.FindAllStringSubmatch(code, -1) {
		decls = append(decls, declaration{name: m[1], kind: "interface", extends: names(m[2])})
	}
	for _, m := range phpTrait.FindAllStringSubmatch(code, -1) {
		decls = append(decls, declaration{name: m[1], kind: "trait"})
	}
	for _, m := range phpEnum.FindAllStringSubmatch(code, -1) {
		decls = append(decls, declaration{name: m[1], kind: "enum", implements: names(m[2])})
	}
	if len(decls) > 0 {
		var traits []string
		for _, m := range phpTraitUse.FindAllStringSubmatch(code, -1) {
			traits = append(traits, names(m[1])...)
		}
		decls[0].uses = traits
	} else {
		for _, m := range phpFunction.FindAllStringSubmatch(code, -1) {
			decls = append(decls, declaration{name: m[1], kind: "function"})
		}
	}

	return parsed{declarations: decls, identifiers: identifiers(stripPHPUseStatements(code))}
}

// stripPHPUseStatements garde le dernier élément des « use A\B\C; » (le nom court est cité).
func stripPHPUseStatements(code string) string {
	return regexp.MustCompile(`[\w]+\\`).ReplaceAllString(code, "")
}

// parseScript : JavaScript et TypeScript.
func parseScript(source string) parsed {
	code := clean(source, false)
	var decls []declaration

	for _, m := range jsClass.FindAllStringSubmatch(code, -1) {
		decls = append(decls, declaration{name: m[1], kind: "class", extends: names(m[2]), implements: names(m[3])})
	}
	for _, m := range tsInterface.FindAllStringSubmatch(code, -1) {
		decls = append(decls, declaration{name: m[1], kind: "interface", extends: names(m[2])})
	}
	for _, m := range tsEnum.FindAllStringSubmatch(code, -1) {
		decls = append(decls, declaration{name: m[1], kind: "enum"})
	}
	if len(decls) == 0 {
		for _, re := range []*regexp.Regexp{jsFunction, jsArrow} {
			for _, m := range re.FindAllStringSubmatch(code, -1) {
				decls = append(decls, declaration{name: m[1], kind: "function"})
			}
		}
	}

	ids := identifiers(code)
	// Fonctions importées en minuscules : « import { createInvoice } from './invoices' ».
	for _, m := range regexp.MustCompile(`import\s*\{([^}]*)\}`).FindAllStringSubmatch(code, -1) {
		for _, n := range strings.Split(m[1], ",") {
			n = strings.TrimSpace(strings.SplitN(strings.TrimSpace(n), " as ", 2)[0])
			if n != "" {
				ids[n] = true
			}
		}
	}

	return parsed{declarations: decls, identifiers: ids}
}

// parseDart : classes, mixins (traits), enums.
func parseDart(source string) parsed {
	code := clean(source, false)
	var decls []declaration

	for _, m := range dartClass.FindAllStringSubmatch(code, -1) {
		decls = append(decls, declaration{name: m[1], kind: "class", extends: names(m[2]), uses: names(m[3]), implements: names(m[4])})
	}
	for _, m := range dartMixin.FindAllStringSubmatch(code, -1) {
		decls = append(decls, declaration{name: m[1], kind: "trait"})
	}
	for _, m := range dartEnum.FindAllStringSubmatch(code, -1) {
		decls = append(decls, declaration{name: m[1], kind: "enum"})
	}

	return parsed{declarations: decls, identifiers: identifiers(code)}
}

// Package diff calcule la différence ligne par ligne entre deux textes (algorithme de Myers),
// présentée en blocs avec 3 lignes de contexte, comme « diff -u ». Bibliothèque standard seule.
package diff

import (
	"fmt"
	"strings"
)

// Kind d'une ligne.
type Kind int

const (
	Same Kind = iota
	Removed
	Added
)

// Line : une ligne de la différence.
type Line struct {
	Kind Kind
	Text string
}

// Hunk : bloc de lignes changées avec leur contexte ; From/To : première ligne (1-based).
type Hunk struct {
	FromLine, ToLine int
	Lines            []Line
}

// MaxLines : au-delà, la comparaison complète n'est pas calculée (fichiers trop longs).
const MaxLines = 20000

// Lines découpe un texte en lignes ; CRLF et LF sont comparés à l'identique.
func Lines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}

	return strings.Split(text, "\n")
}

// Compute renvoie les blocs de différence entre a (avant) et b (après), avec context lignes autour.
func Compute(a, b []string, context int) ([]Hunk, error) {
	if len(a) > MaxLines || len(b) > MaxLines {
		return nil, fmt.Errorf("files too long to compare")
	}

	ops := myers(a, b)

	var hunks []Hunk
	for i := 0; i < len(ops); {
		if ops[i].Kind == Same {
			i++
			continue
		}
		start := max(0, i-context)
		end := i
		// Étend le bloc tant que deux changements sont séparés de moins de 2×context lignes.
		for end < len(ops) {
			if ops[end].Kind != Same {
				end++
				continue
			}
			run := end
			for run < len(ops) && ops[run].Kind == Same {
				run++
			}
			if run == len(ops) || run-end > 2*context {
				end = min(len(ops), end+context)
				break
			}
			end = run
		}

		hunk := Hunk{FromLine: ops[start].aLine, ToLine: ops[start].bLine}
		for _, op := range ops[start:end] {
			hunk.Lines = append(hunk.Lines, Line{Kind: op.Kind, Text: op.text})
		}
		hunks = append(hunks, hunk)
		i = end
	}

	return hunks, nil
}

type op struct {
	Kind         Kind
	text         string
	aLine, bLine int // numéros de ligne (1-based) au début de l'opération
}

// myers : plus court script d'édition (Myers 1986), puis remontée des étapes.
func myers(a, b []string) []op {
	n, m := len(a), len(b)
	maxD := n + m
	offset := maxD + 1
	v := make([]int, 2*maxD+3)
	var trace [][]int

	for d := 0; d <= maxD; d++ {
		trace = append(trace, append([]int(nil), v...))
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
				x = v[offset+k+1]
			} else {
				x = v[offset+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[offset+k] = x
			if x >= n && y >= m {
				return backtrack(trace, a, b, offset, d)
			}
		}
	}

	return nil
}

func backtrack(trace [][]int, a, b []string, offset, d int) []op {
	x, y := len(a), len(b)
	var reversed []op

	for ; d >= 0; d-- {
		v := trace[d]
		k := x - y
		var prevK int
		if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := v[offset+prevK]
		prevY := prevX - prevK

		for x > prevX && y > prevY {
			x--
			y--
			reversed = append(reversed, op{Kind: Same, text: a[x], aLine: x + 1, bLine: y + 1})
		}
		if d > 0 {
			if x == prevX {
				y--
				reversed = append(reversed, op{Kind: Added, text: b[y], aLine: x + 1, bLine: y + 1})
			} else {
				x--
				reversed = append(reversed, op{Kind: Removed, text: a[x], aLine: x + 1, bLine: y + 1})
			}
		}
	}

	ops := make([]op, len(reversed))
	for i, o := range reversed {
		ops[len(reversed)-1-i] = o
	}

	return ops
}

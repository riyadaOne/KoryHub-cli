// Package semver compare des versions au format x.y.z (versions du CLI, T-11).
package semver

import (
	"fmt"
	"strconv"
	"strings"
)

// Version est une version x.y.z.
type Version struct {
	Major, Minor, Patch int
}

// Parse lit une version x.y.z ; « 1.0 » est refusé (piège 12 des projets précédents).
func Parse(s string) (Version, error) {
	parts := strings.Split(strings.TrimSpace(s), ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("version %q: format x.y.z attendu", s)
	}

	numbers := make([]int, 3)
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || part == "" {
			return Version{}, fmt.Errorf("version %q: format x.y.z attendu", s)
		}
		numbers[i] = n
	}

	return Version{numbers[0], numbers[1], numbers[2]}, nil
}

// Compare renvoie -1, 0 ou 1 selon que v est inférieure, égale ou supérieure à other.
func (v Version) Compare(other Version) int {
	for _, pair := range [][2]int{{v.Major, other.Major}, {v.Minor, other.Minor}, {v.Patch, other.Patch}} {
		switch {
		case pair[0] < pair[1]:
			return -1
		case pair[0] > pair[1]:
			return 1
		}
	}

	return 0
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Package version porte la version du CLI, injectée à la construction (TRD §18, T-11).
package version

import (
	"fmt"
	"runtime"
)

// Version du CLI au format x.y.z, remplacée à la construction par
// -ldflags "-X koryhub.dev/kory/internal/version.Version=1.2.3".
var Version = "0.1.0"

// UserAgent est l'en-tête par lequel le CLI s'annonce à l'API : version, système
// et architecture, rien d'autre (aucune information sur le projet ni la machine, T-44).
func UserAgent() string {
	return fmt.Sprintf("kory/%s (%s; %s)", Version, runtime.GOOS, runtime.GOARCH)
}

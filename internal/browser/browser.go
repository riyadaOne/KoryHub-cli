// Package browser ouvre une adresse dans le navigateur du système (K01) : commande du
// système sous Windows, macOS et Linux. Si elle échoue, l'appelant affiche l'adresse.
package browser

import (
	"os/exec"
	"runtime"
)

// Open ouvre url ; remplaçable dans les tests.
var Open = func(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	return cmd.Start()
}

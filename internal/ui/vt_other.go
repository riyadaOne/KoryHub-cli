//go:build !windows

package ui

import "io"

// enableVirtualTerminal : rien à faire hors de Windows, les terminaux comprennent l'ANSI.
func enableVirtualTerminal(io.Writer) {}

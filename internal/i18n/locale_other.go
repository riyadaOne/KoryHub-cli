//go:build !windows

package i18n

import "os"

// systemLocale lit la langue du système dans les variables habituelles de macOS et Linux.
func systemLocale() string {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := os.Getenv(name); value != "" && value != "C" && value != "POSIX" {
			return value
		}
	}

	return ""
}

//go:build windows

package i18n

import (
	"os"
	"syscall"
	"unsafe"
)

// systemLocale lit la langue de Windows (GetUserDefaultLocaleName), sauf si LANG est défini.
func systemLocale() string {
	if lang := os.Getenv("LANG"); lang != "" {
		return lang
	}

	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultLocaleName")
	if proc.Find() != nil {
		return ""
	}

	buffer := make([]uint16, 85) // LOCALE_NAME_MAX_LENGTH
	length, _, _ := proc.Call(uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	if length == 0 {
		return ""
	}

	return syscall.UTF16ToString(buffer)
}

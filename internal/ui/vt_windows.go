//go:build windows

package ui

import (
	"io"
	"os"
	"syscall"
	"unsafe"
)

// enableVirtualTerminal active les couleurs ANSI dans la console Windows 10 et plus
// (ENABLE_VIRTUAL_TERMINAL_PROCESSING) ; sans effet si la console ne le permet pas.
func enableVirtualTerminal(w io.Writer) {
	file, ok := w.(*os.File)
	if !ok {
		return
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getMode := kernel32.NewProc("GetConsoleMode")
	setMode := kernel32.NewProc("SetConsoleMode")

	var mode uint32
	if ret, _, _ := getMode.Call(file.Fd(), uintptr(unsafe.Pointer(&mode))); ret == 0 {
		return
	}

	const enableVirtualTerminalProcessing = 0x0004
	setMode.Call(file.Fd(), uintptr(mode|enableVirtualTerminalProcessing))
}

package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// Windows MessageBox constants
const (
	mbOK              uint32 = 0x00000000
	mbIconError       uint32 = 0x00000010
	mbIconInformation uint32 = 0x00000040
)

var (
	user32DLL  = syscall.NewLazyDLL("user32.dll")
	messageBox = user32DLL.NewProc("MessageBoxW")
)

// showMessage displays a native Windows message box.
func showMessage(title, msg string, flags uint32) {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(msg)
	messageBox.Call(
		0,
		uintptr(unsafe.Pointer(m)),
		uintptr(unsafe.Pointer(t)),
		uintptr(flags),
	)
}

func main() {
	args := os.Args[1:]

	// ── Call mode: Windows launched us with a tel: URI ────────────────────────
	if len(args) == 1 && strings.HasPrefix(strings.ToLower(args[0]), "tel:") {
		cfg, err := LoadConfig()
		if err != nil {
			showMessage("FpbxCTC", "Failed to load config:\n"+err.Error(), mbOK|mbIconError)
			os.Exit(1)
		}
		if cfg.Domain == "" || cfg.APIKey == "" || cfg.AgentNumber == "" {
			showMessage(
				"FpbxCTC",
				"Settings not configured.\nRun FpbxCTC without arguments to open Settings.",
				mbOK|mbIconError,
			)
			os.Exit(1)
		}

		dest := sanitizeNumber(args[0])
		if err := MakeCall(cfg, args[0]); err != nil {
			showMessage("FpbxCTC", "Call failed:\n"+err.Error(), mbOK|mbIconError)
			os.Exit(1)
		}

		showMessage(
			"FpbxCTC",
			fmt.Sprintf("Calling %s\nAgent: %s", dest, cfg.AgentNumber),
			mbOK|mbIconInformation,
		)
		return
	}

	// ── Settings mode: no tel: argument ──────────────────────────────────────
	RunSettings()
}

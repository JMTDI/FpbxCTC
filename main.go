package main

import (
	"encoding/binary"
	"encoding/json"
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

	// ── Native messaging mode: Chrome/Edge/Brave launched us ─────────────────
	// The browser passes the calling extension's origin as the sole argument,
	// e.g. "chrome-extension://mbabhkdiiiceedngdpgbifgnabaaboeb/"
	// It does NOT pass --native-messaging; we detect by the URL scheme.
	if len(args) >= 1 && strings.HasPrefix(strings.ToLower(args[0]), "chrome-extension://") {
		handleNativeMessaging()
		return
	}

	// ── Silent install/uninstall hooks (called by installer/uninstaller) ──────
	if len(args) >= 1 {
		switch args[0] {
		case "-register":
			RegisterProtocol() //nolint:errcheck
			return
		case "-unregister":
			UnregisterProtocol() //nolint:errcheck
			return
		case "-uninstall-extension":
			RunBrowserUninstall(func(string) {})
			return
		}
	}

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

// handleNativeMessaging implements the Chrome Native Messaging protocol:
// read a 4-byte LE length-prefixed JSON message from stdin, respond with
// the current config (or an error), then exit.
func handleNativeMessaging() {
	// Read the 4-byte length prefix
	var msgLen uint32
	if err := binary.Read(os.Stdin, binary.LittleEndian, &msgLen); err != nil {
		nmRespond(map[string]string{"error": "read length: " + err.Error()})
		return
	}
	if msgLen > 1024*1024 {
		nmRespond(map[string]string{"error": "message too large"})
		return
	}

	// Read the JSON message (we don't need its contents — any message = get config)
	buf := make([]byte, msgLen)
	if _, err := os.Stdin.Read(buf); err != nil {
		nmRespond(map[string]string{"error": "read body: " + err.Error()})
		return
	}

	cfg, err := LoadConfig()
	if err != nil {
		nmRespond(map[string]string{"error": err.Error()})
		return
	}
	nmRespond(map[string]string{
		"domain":      cfg.Domain,
		"apiKey":      cfg.APIKey,
		"agentNumber": cfg.AgentNumber,
	})
}

func nmRespond(v any) {
	data, _ := json.Marshal(v)
	binary.Write(os.Stdout, binary.LittleEndian, uint32(len(data))) //nolint:errcheck
	os.Stdout.Write(data)                                           //nolint:errcheck
}

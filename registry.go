package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// exePath returns the absolute path to the running executable.
func exePath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Abs(p)
}

// deleteKeyTree recursively deletes a registry key and all its subkeys.
// Errors are silently ignored so that a partial uninstall always completes.
func deleteKeyTree(root registry.Key, path string) {
	k, err := registry.OpenKey(root, path, registry.ALL_ACCESS)
	if err != nil {
		return
	}
	names, _ := k.ReadSubKeyNames(-1)
	k.Close()
	for _, name := range names {
		deleteKeyTree(root, path+`\`+name)
	}
	registry.DeleteKey(root, path) //nolint:errcheck
}

// RegisterProtocol writes all four registry sections that Windows needs to
// list FpbxCTC in Settings → Default Apps → tel protocol, and also writes
// the Native Messaging host manifest for Chrome and Edge.
func RegisterProtocol() error {
	exe, err := exePath()
	if err != nil {
		return fmt.Errorf("cannot determine exe path: %w", err)
	}
	// Shell-open command: "C:\...\FpbxCTC.exe" "%1"
	cmd := fmt.Sprintf(`"%s" "%%1"`, exe)

	if err := writeProgID(cmd); err != nil {
		return err
	}
	if err := writeTelClass(cmd); err != nil {
		return err
	}
	if err := writeCapabilities(); err != nil {
		return err
	}
	if err := writeRegisteredApp(); err != nil {
		return err
	}
	return writeNativeMessagingHost(exe)
}

// writeProgID creates HKCU\Software\Classes\FpbxCTC.tel
func writeProgID(cmd string) error {
	k, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		`Software\Classes\FpbxCTC.tel`,
		registry.ALL_ACCESS,
	)
	if err != nil {
		return fmt.Errorf("create ProgID key: %w", err)
	}
	defer k.Close()

	if err := k.SetStringValue("", "FpbxCTC Click to Call"); err != nil {
		return err
	}
	if err := k.SetStringValue("URL Protocol", ""); err != nil {
		return err
	}

	open, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		`Software\Classes\FpbxCTC.tel\shell\open\command`,
		registry.ALL_ACCESS,
	)
	if err != nil {
		return fmt.Errorf("create ProgID command key: %w", err)
	}
	defer open.Close()
	return open.SetStringValue("", cmd)
}

// writeTelClass creates HKCU\Software\Classes\tel (direct URI handler).
func writeTelClass(cmd string) error {
	k, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		`Software\Classes\tel`,
		registry.ALL_ACCESS,
	)
	if err != nil {
		return fmt.Errorf("create tel class key: %w", err)
	}
	defer k.Close()

	if err := k.SetStringValue("", "URL:Telephone Protocol"); err != nil {
		return err
	}
	if err := k.SetStringValue("URL Protocol", ""); err != nil {
		return err
	}

	open, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		`Software\Classes\tel\shell\open\command`,
		registry.ALL_ACCESS,
	)
	if err != nil {
		return fmt.Errorf("create tel command key: %w", err)
	}
	defer open.Close()
	return open.SetStringValue("", cmd)
}

// writeCapabilities creates HKCU\Software\FpbxCTC\Capabilities
func writeCapabilities() error {
	k, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		`Software\FpbxCTC\Capabilities`,
		registry.ALL_ACCESS,
	)
	if err != nil {
		return fmt.Errorf("create Capabilities key: %w", err)
	}
	defer k.Close()

	if err := k.SetStringValue("ApplicationName", "FpbxCTC"); err != nil {
		return err
	}
	if err := k.SetStringValue("ApplicationDescription", "FreePBX Click to Call"); err != nil {
		return err
	}

	ua, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		`Software\FpbxCTC\Capabilities\URLAssociations`,
		registry.ALL_ACCESS,
	)
	if err != nil {
		return fmt.Errorf("create URLAssociations key: %w", err)
	}
	defer ua.Close()
	return ua.SetStringValue("tel", "FpbxCTC.tel")
}

// writeRegisteredApp adds FpbxCTC to HKCU\Software\RegisteredApplications
func writeRegisteredApp() error {
	k, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		`Software\RegisteredApplications`,
		registry.ALL_ACCESS,
	)
	if err != nil {
		return fmt.Errorf("create RegisteredApplications key: %w", err)
	}
	defer k.Close()
	return k.SetStringValue("FpbxCTC", `Software\FpbxCTC\Capabilities`)
}

// UnregisterProtocol removes every registry key created by RegisterProtocol.
func UnregisterProtocol() error {
	deleteKeyTree(registry.CURRENT_USER, `Software\Classes\FpbxCTC.tel`)
	deleteKeyTree(registry.CURRENT_USER, `Software\Classes\tel`)
	deleteKeyTree(registry.CURRENT_USER, `Software\FpbxCTC`)

	ra, err := registry.OpenKey(
		registry.CURRENT_USER,
		`Software\RegisteredApplications`,
		registry.ALL_ACCESS,
	)
	if err == nil {
		ra.DeleteValue("FpbxCTC") //nolint:errcheck
		ra.Close()
	}
	removeNativeMessagingHost()
	return nil
}

// registerNMHostOnly writes the Native Messaging host manifest and registry
// entries without touching the tel: protocol registration. Called from the
// browser extension install flow so sync works immediately after install.
func registerNMHostOnly() error {
	exe, err := exePath()
	if err != nil {
		return fmt.Errorf("cannot determine exe path: %w", err)
	}
	return writeNativeMessagingHost(exe)
}

// ── Native Messaging Host ─────────────────────────────────────────────────────

// nmHostName is the identifier used in the NM manifest and registry.
const nmHostName = "com.fpbxctc.host"

// nmExtensionID is the fixed Chrome/Edge extension ID.
//
// manifest.json intentionally has no "key" field: Microsoft Edge can fail to
// load unpacked extensions that ship a Chrome-generated "key" (the extension
// shows as corrupted / refuses to load), so relying on it broke Edge support.
// Without "key", Chromium-based browsers (Chrome, Edge, Brave, Vivaldi all
// share the same extensions code) derive the ID deterministically from the
// lower-cased absolute path of the unpacked extension folder. Since the
// installer always places the extension at the same fixed path
// (C:\Program Files\FpbxCTC\browser-extension), the resulting ID below is
// stable across every supported browser and every install.
const nmExtensionID = "chrome-extension://dicdjpbfhifgdnhgbbbnbenocgeegjij/"

const edgeExtensionIDFile = "edge-extension-id.txt"

type nmManifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

// nmInstalledExe returns the canonical installed exe path, using the
// ProgramFiles environment variable so it adapts to non-default installs.
func nmInstalledExe() string {
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		pf = `C:\Program Files`
	}
	return filepath.Join(pf, "FpbxCTC", "FpbxCTC.exe")
}

func edgeExtensionIDPath() string {
	return filepath.Join(configDir(), edgeExtensionIDFile)
}

func savedEdgeExtensionID() string {
	data, err := os.ReadFile(edgeExtensionIDPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func saveEdgeExtensionID(id string) error {
	id = strings.TrimSpace(strings.TrimPrefix(id, "chrome-extension://"))
	id = strings.TrimSuffix(id, "/")
	if len(id) != 32 {
		return fmt.Errorf("Edge extension ID must be 32 characters")
	}
	for _, char := range id {
		if char < 'a' || char > 'p' {
			return fmt.Errorf("Edge extension ID contains invalid characters")
		}
	}
	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	return os.WriteFile(edgeExtensionIDPath(), []byte(id+"\n"), 0o600)
}

func nativeMessagingOrigins() []string {
	origins := []string{nmExtensionID}
	if id := savedEdgeExtensionID(); id != "" {
		origins = append(origins, "chrome-extension://"+id+"/")
	}
	return origins
}

func writeNativeMessagingHost(exeFullPath string) error {
	// Always prefer the installed exe so that the NM host path is stable
	// even when the user runs the dev build to reconfigure settings.
	if installed := nmInstalledExe(); installed != "" {
		if _, err := os.Stat(installed); err == nil {
			exeFullPath = installed
		}
	}

	manifest := nmManifest{
		Name:           nmHostName,
		Description:    "FpbxCTC native messaging host",
		Path:           exeFullPath,
		Type:           "stdio",
		AllowedOrigins: nativeMessagingOrigins(),
	}

	// Write manifest JSON to %APPDATA%\FpbxCTC\
	dir := filepath.Join(os.Getenv("APPDATA"), "FpbxCTC")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("nm mkdir: %w", err)
	}
	manifestPath := filepath.Join(dir, "fpbxctc_nm.json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		return fmt.Errorf("nm write manifest: %w", err)
	}

	// Register for all Chromium-based browsers
	for _, keyPath := range []string{
		`SOFTWARE\Google\Chrome\NativeMessagingHosts\` + nmHostName,
		`SOFTWARE\Microsoft\Edge\NativeMessagingHosts\` + nmHostName,
		`SOFTWARE\BraveSoftware\Brave-Browser\NativeMessagingHosts\` + nmHostName,
		`SOFTWARE\Vivaldi\NativeMessagingHosts\` + nmHostName,
		`SOFTWARE\Chromium\NativeMessagingHosts\` + nmHostName,
	} {
		if err := writeNMRegistry(keyPath, manifestPath); err != nil {
			return err
		}
	}
	return nil
}

func writeNMRegistry(keyPath, manifestPath string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("nm registry %s: %w", keyPath, err)
	}
	defer k.Close()
	return k.SetStringValue("", manifestPath)
}

func removeNativeMessagingHost() {
	manifestPath := filepath.Join(os.Getenv("APPDATA"), "FpbxCTC", "fpbxctc_nm.json")
	os.Remove(manifestPath) //nolint:errcheck
	for _, keyPath := range []string{
		`SOFTWARE\Google\Chrome\NativeMessagingHosts\` + nmHostName,
		`SOFTWARE\Microsoft\Edge\NativeMessagingHosts\` + nmHostName,
		`SOFTWARE\BraveSoftware\Brave-Browser\NativeMessagingHosts\` + nmHostName,
		`SOFTWARE\Vivaldi\NativeMessagingHosts\` + nmHostName,
		`SOFTWARE\Chromium\NativeMessagingHosts\` + nmHostName,
	} {
		deleteKeyTree(registry.CURRENT_USER, keyPath)
	}
}

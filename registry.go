package main

import (
	"fmt"
	"os"
	"path/filepath"

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
// list FpbxCTC in Settings → Default Apps → tel protocol.
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
	return writeRegisteredApp()
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
	return nil
}

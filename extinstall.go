package main

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed browser-extension.zip
var extensionZIP []byte

// extensionDir returns the path where the extension is extracted.
func extensionDir() string {
	return filepath.Join(os.Getenv("APPDATA"), "FpbxCTC", "extension")
}

// bootstrapConfig is written into the extracted extension so popup.js can
// auto-populate settings on first load.
type bootstrapConfig struct {
	Domain      string `json:"domain"`
	APIKey      string `json:"api_key"`
	AgentNumber string `json:"agent_number"`
}

// ExtractExtension unzips the embedded browser-extension.zip to
// %APPDATA%\FpbxCTC\extension\ and writes bootstrap.json with the current cfg.
func ExtractExtension(cfg *Config) error {
	dir := extensionDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	r, err := zip.NewReader(bytes.NewReader(extensionZIP), int64(len(extensionZIP)))
	if err != nil {
		return err
	}

	for _, f := range r.File {
		dest := filepath.Join(dir, filepath.FromSlash(f.Name))
		if f.FileInfo().IsDir() {
			os.MkdirAll(dest, 0o755) //nolint:errcheck
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(dest)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}

	// Write bootstrap.json so popup.js auto-fills settings on first open
	bs := bootstrapConfig{
		Domain:      cfg.Domain,
		APIKey:      cfg.APIKey,
		AgentNumber: cfg.AgentNumber,
	}
	data, err := json.MarshalIndent(bs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "bootstrap.json"), data, 0o600)
}

// DetectBrowsers returns the paths of Chrome and Edge if found.
func DetectBrowsers() (chrome, edge string) {
	candidates := map[string]*string{
		`C:\Program Files\Google\Chrome\Application\chrome.exe`:          &chrome,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`:    &chrome,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`:         &edge,
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`:   &edge,
	}
	for path, dst := range candidates {
		if _, err := os.Stat(path); err == nil {
			*dst = path
		}
	}
	return
}

// OpenExtensionsPage launches the given browser at its extensions management page.
func OpenExtensionsPage(browserExe string) error {
	if browserExe == "" {
		return errors.New("browser not found")
	}
	var page string
	switch {
	case isEdge(browserExe):
		page = "edge://extensions"
	default:
		page = "chrome://extensions"
	}
	return exec.Command(browserExe, "--new-window", page).Start()
}

func isEdge(exe string) bool {
	base := filepath.Base(exe)
	return base == "msedge.exe"
}

package main

import (
	"bytes"
	_ "embed"
	"image/png"
	"os/exec"
	"regexp"
	"strings"

	"github.com/gonutz/wui/v2"
)

//go:embed FpbxCTC.png
var appIconPNG []byte

func RunSettings() {
	cfg, _ := LoadConfig()
	if cfg == nil {
		cfg = &Config{}
	}

	font, _ := wui.NewFont(wui.FontDesc{Name: "Segoe UI", Height: -14})

	w := wui.NewWindow()
	w.SetTitle("FpbxCTC \u2014 Settings")
	w.SetWidth(516)
	w.SetHeight(470)
	w.SetHasMaxButton(false)
	if font != nil {
		w.SetFont(font)
	}

	// Set window/taskbar icon from the embedded PNG
	if img, err := png.Decode(bytes.NewReader(appIconPNG)); err == nil {
		if icon, err := wui.NewIconFromImage(img); err == nil {
			w.SetIcon(icon)
		}
	}

	// ── Labels ────────────────────────────────────────────────────────────────

	addLabel := func(text string, x, y, width, height int) {
		l := wui.NewLabel()
		l.SetText(text)
		l.SetBounds(x, y, width, height)
		w.Add(l)
	}

	addLabel("Domain:", 10, 23, 115, 20)
	addLabel("API Key:", 10, 62, 115, 20)
	addLabel("Agent Number:", 10, 101, 115, 20)
	addLabel("Domain = without https://   |   Agent = your desk extension", 10, 130, 480, 18)

	// ── Edit fields ───────────────────────────────────────────────────────────

	domainEdit := wui.NewEditLine()
	domainEdit.SetText(cfg.Domain)
	domainEdit.SetBounds(130, 18, 360, 26)
	w.Add(domainEdit)

	apiEdit := wui.NewEditLine()
	apiEdit.SetText(cfg.APIKey)
	apiEdit.SetIsPassword(true)
	apiEdit.SetBounds(130, 57, 360, 26)
	w.Add(apiEdit)

	agentEdit := wui.NewEditLine()
	agentEdit.SetText(cfg.AgentNumber)
	agentEdit.SetBounds(130, 96, 360, 26)
	w.Add(agentEdit)

	// ── Status bar ────────────────────────────────────────────────────────────

	statusLabel := wui.NewLabel()
	statusLabel.SetBounds(10, 330, 480, 22)
	w.Add(statusLabel)

	setStatus := func(msg string) {
		statusLabel.SetText(msg)
	}

	// ── Buttons ───────────────────────────────────────────────────────────────

	saveBtn := wui.NewButton()
	saveBtn.SetText("Save Settings")
	saveBtn.SetBounds(10, 158, 480, 32)
	saveBtn.SetOnClick(func() {
		// Strip non-digits from agent number
		agentRaw := strings.TrimSpace(agentEdit.Text())
		agentClean := regexp.MustCompile(`[^\d]`).ReplaceAllString(agentRaw, "")
		agentEdit.SetText(agentClean)

		c := &Config{
			Domain:      domainEdit.Text(),
			APIKey:      apiEdit.Text(),
			AgentNumber: agentClean,
		}
		if err := SaveConfig(c); err != nil {
			setStatus("Save failed: " + err.Error())
			return
		}
		setStatus("Settings saved.")
	})
	w.Add(saveBtn)

	regBtn := wui.NewButton()
	regBtn.SetText("Register as tel: handler")
	regBtn.SetBounds(10, 202, 232, 32)
	regBtn.SetOnClick(func() {
		if err := RegisterProtocol(); err != nil {
			setStatus("Registration failed: " + err.Error())
			return
		}
		setStatus("Registered! On Windows 11 click 'Open Windows Default Apps' to confirm.")
	})
	w.Add(regBtn)

	unregBtn := wui.NewButton()
	unregBtn.SetText("Unregister")
	unregBtn.SetBounds(252, 202, 238, 32)
	unregBtn.SetOnClick(func() {
		if err := UnregisterProtocol(); err != nil {
			setStatus("Unregister failed: " + err.Error())
			return
		}
		setStatus("Unregistered.")
	})
	w.Add(unregBtn)

	defAppsBtn := wui.NewButton()
	defAppsBtn.SetText("Open Windows Default Apps  (required on Windows 11)")
	defAppsBtn.SetBounds(10, 246, 480, 32)
	defAppsBtn.SetOnClick(func() {
		exec.Command("cmd", "/c", "start", "ms-settings:defaultapps").Start() //nolint:errcheck
		setStatus("Opened Windows Default Apps.")
	})
	w.Add(defAppsBtn)

	// ── Install Browser Extension ─────────────────────────────────────────────

	extBtn := wui.NewButton()
	extBtn.SetText("Install Browser Extension")
	extBtn.SetBounds(10, 292, 480, 32)
	extBtn.SetOnClick(func() {
		// Load latest saved config so bootstrap.json gets current values
		latestCfg := &Config{
			Domain:      domainEdit.Text(),
			APIKey:      apiEdit.Text(),
			AgentNumber: agentEdit.Text(),
		}
		if err := ExtractExtension(latestCfg); err != nil {
			setStatus("Extract failed: " + err.Error())
			return
		}
		showExtensionInstallWindow(latestCfg)
	})
	w.Add(extBtn)

	extDir := wui.NewLabel()
	extDir.SetText("Extracts extension to %APPDATA%\\FpbxCTC\\extension\\ for Chrome & Edge")
	extDir.SetBounds(10, 330, 480, 18)
	w.Add(extDir)

	statusLabel.SetBounds(10, 410, 480, 22)

	w.Show()
}

// showExtensionInstallWindow opens a small popup showing Chrome/Edge detection
// and load-unpacked instructions.
func showExtensionInstallWindow(_ *Config) {
	chrome, edge := DetectBrowsers()
	dir := extensionDir()

	font, _ := wui.NewFont(wui.FontDesc{Name: "Segoe UI", Height: -14})

	w2 := wui.NewWindow()
	w2.SetTitle("Install Browser Extension")
	w2.SetWidth(500)
	w2.SetHeight(320)
	w2.SetHasMaxButton(false)
	w2.SetHasMinButton(false)
	if font != nil {
		w2.SetFont(font)
	}
	if img, err := png.Decode(bytes.NewReader(appIconPNG)); err == nil {
		if icon, err := wui.NewIconFromImage(img); err == nil {
			w2.SetIcon(icon)
		}
	}

	addLabel2 := func(text string, x, y, w, h int) {
		l := wui.NewLabel()
		l.SetText(text)
		l.SetBounds(x, y, w, h)
		w2.Add(l)
	}

	addLabel2("Extension folder:", 10, 12, 120, 20)
	addLabel2(dir, 135, 12, 345, 20)

	addLabel2("Steps:  1. Click a browser button below", 10, 40, 470, 20)
	addLabel2("        2. Enable Developer mode (top-right toggle)", 10, 60, 470, 20)
	addLabel2("        3. Click Load unpacked", 10, 80, 470, 20)
	addLabel2("        4. Select the folder shown above", 10, 100, 470, 20)
	addLabel2("Settings are pre-filled automatically.", 10, 122, 470, 20)

	statusL := wui.NewLabel()
	statusL.SetBounds(10, 270, 470, 22)
	w2.Add(statusL)
	setStatus2 := func(msg string) { statusL.SetText(msg) }

	// Chrome button
	chromeBtn := wui.NewButton()
	if chrome != "" {
		chromeBtn.SetText("Open Chrome Extensions Page")
	} else {
		chromeBtn.SetText("Chrome not found")
		chromeBtn.SetEnabled(false)
	}
	chromeBtn.SetBounds(10, 152, 228, 36)
	chromeBtn.SetOnClick(func() {
		if err := OpenExtensionsPage(chrome); err != nil {
			setStatus2("Could not open Chrome: " + err.Error())
			return
		}
		setStatus2("Chrome opened — follow the steps above.")
	})
	w2.Add(chromeBtn)

	// Edge button
	edgeBtn := wui.NewButton()
	if edge != "" {
		edgeBtn.SetText("Open Edge Extensions Page")
	} else {
		edgeBtn.SetText("Edge not found")
		edgeBtn.SetEnabled(false)
	}
	edgeBtn.SetBounds(252, 152, 228, 36)
	edgeBtn.SetOnClick(func() {
		if err := OpenExtensionsPage(edge); err != nil {
			setStatus2("Could not open Edge: " + err.Error())
			return
		}
		setStatus2("Edge opened — follow the steps above.")
	})
	w2.Add(edgeBtn)

	// Open folder button
	folderBtn := wui.NewButton()
	folderBtn.SetText("Open Extension Folder in Explorer")
	folderBtn.SetBounds(10, 200, 470, 32)
	folderBtn.SetOnClick(func() {
		exec.Command("explorer", dir).Start() //nolint:errcheck
	})
	w2.Add(folderBtn)

	w2.Show()
}


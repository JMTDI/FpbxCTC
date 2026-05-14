package main

import (
	"bytes"
	_ "embed"
	"image/png"

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
	w.SetHeight(380)
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
	statusLabel.SetBounds(10, 320, 480, 22)
	w.Add(statusLabel)

	setStatus := func(msg string) {
		statusLabel.SetText(msg)
	}

	// ── Buttons ───────────────────────────────────────────────────────────────

	saveBtn := wui.NewButton()
	saveBtn.SetText("Save Settings")
	saveBtn.SetBounds(10, 158, 480, 32)
	saveBtn.SetOnClick(func() {
		c := &Config{
			Domain:      domainEdit.Text(),
			APIKey:      apiEdit.Text(),
			AgentNumber: agentEdit.Text(),
		}
		if err := SaveConfig(c); err != nil {
			setStatus("Save failed: " + err.Error())
			return
		}
		setStatus("Settings saved.")
	})
	w.Add(saveBtn)

	extBtn := wui.NewButton()
	extBtn.SetText("Install Browser Extension")
	extBtn.SetBounds(10, 202, 236, 32)
	extBtn.SetOnClick(func() {
		RunBrowserInstall(setStatus)
	})
	w.Add(extBtn)

	unextBtn := wui.NewButton()
	unextBtn.SetText("Uninstall Extension")
	unextBtn.SetBounds(254, 202, 236, 32)
	unextBtn.SetOnClick(func() {
		RunBrowserUninstall(setStatus)
	})
	w.Add(unextBtn)

	w.Show()
}


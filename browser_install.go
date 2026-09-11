package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/gonutz/wui/v2"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// browser holds a detected browser's display name and executable path.
type browser struct {
	name    string
	exePath string
}

func browserInstallLogPath() string {
	return filepath.Join(configDir(), "browser-install.log")
}

func logBrowserInstall(format string, args ...any) {
	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(browserInstallLogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s "+format+"\n", append([]any{time.Now().Format(time.RFC3339)}, args...)...)
}

// detectBrowsers searches well-known registry paths for Chromium-based
// browsers. Only browsers that are actually installed are returned.
func detectBrowsers() []browser {
	candidates := []struct {
		name    string
		regPath string // HKLM path; value "" (default) holds exe path
	}{
		{"Google Chrome", `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\chrome.exe`},
		{"Microsoft Edge", `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\msedge.exe`},
		{"Brave", `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\brave.exe`},
		{"Vivaldi", `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\vivaldi.exe`},
	}

	var found []browser
	for _, c := range candidates {
		foundInRegistry := false
		// Try HKLM then HKCU so per-user installs are also detected.
		for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
			k, err := registry.OpenKey(root, c.regPath, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			path, _, err := k.GetStringValue("")
			k.Close()
			path = strings.Trim(strings.TrimSpace(path), `"`)
			if err != nil || path == "" {
				logBrowserInstall("%s registry entry is empty or invalid: %v", c.name, err)
				continue
			}
			if _, err := os.Stat(path); err != nil {
				logBrowserInstall("%s registry path unavailable: %s: %v", c.name, path, err)
				continue
			}
			found = append(found, browser{name: c.name, exePath: path})
			logBrowserInstall("Detected %s from registry: %s", c.name, path)
			foundInRegistry = true
			break // don't add the same browser twice
		}

		// Edge can be installed per-user without an App Paths entry. Probe its
		// documented machine and per-user locations as a fallback.
		if c.name == "Microsoft Edge" && !foundInRegistry {
			for _, path := range edgeExecutablePaths() {
				if _, err := os.Stat(path); err == nil {
					found = append(found, browser{name: c.name, exePath: path})
					logBrowserInstall("Detected Microsoft Edge from fallback path: %s", path)
					break
				}
			}
		}
	}
	return found
}

func edgeExecutablePaths() []string {
	var result []string
	for _, root := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		if base := os.Getenv(root); base != "" {
			result = append(result, filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"))
		}
	}
	return result
}

// RunBrowserInstall opens a picker window listing installed Chromium-based
// browsers. It opens the selected browser's Extensions page and the bundled
// extension folder for the user-confirmed unpacked installation step.
// Progress / errors are reported via setStatus (called on the GUI thread).
func RunBrowserInstall(setStatus func(string)) {
	logBrowserInstall("Browser extension install started")
	browsers := detectBrowsers()
	if len(browsers) == 0 {
		logBrowserInstall("No supported browser detected")
		setStatus("No supported browser detected (Chrome, Edge, Brave, Vivaldi).")
		return
	}

	font, _ := wui.NewFont(wui.FontDesc{Name: "Segoe UI", Height: -14})

	win := wui.NewWindow()
	win.SetTitle("Install Browser Extension")
	win.SetWidth(420)
	win.SetHeight(276 + len(browsers)*32)
	win.SetHasMaxButton(false)
	if font != nil {
		win.SetFont(font)
	}

	infoLabel := wui.NewLabel()
	infoLabel.SetText("Select a browser to load the FpbxCTC extension:")
	infoLabel.SetBounds(12, 14, 390, 20)
	win.Add(infoLabel)

	// Radio buttons — one per detected browser
	radios := make([]*wui.RadioButton, len(browsers))
	for i, b := range browsers {
		rb := wui.NewRadioButton()
		rb.SetText(b.name)
		rb.SetBounds(12, 44+i*32, 390, 26)
		if i == 0 {
			rb.SetChecked(true)
		}
		win.Add(rb)
		radios[i] = rb
	}

	noteLabel := wui.NewLabel()
	noteLabel.SetText("For Edge sync: after Load unpacked, paste its ID below and click Install again.")
	noteLabel.SetBounds(12, 50+len(browsers)*32, 390, 36)
	win.Add(noteLabel)

	edgeIDLabel := wui.NewLabel()
	edgeIDLabel.SetText("Edge extension ID (optional):")
	edgeIDLabel.SetBounds(12, 90+len(browsers)*32, 390, 20)
	win.Add(edgeIDLabel)

	edgeIDEdit := wui.NewEditLine()
	edgeIDEdit.SetText(savedEdgeExtensionID())
	edgeIDEdit.SetBounds(12, 112+len(browsers)*32, 390, 26)
	win.Add(edgeIDEdit)

	installBtn := wui.NewButton()
	installBtn.SetText("Install Extension")
	installBtn.SetBounds(12, 156+len(browsers)*32, 192, 32)
	installBtn.SetOnClick(func() {
		idx := 0
		for i, rb := range radios {
			if rb.Checked() {
				idx = i
				break
			}
		}
		chosen := browsers[idx]
		logBrowserInstall("Selected %s: %s", chosen.name, chosen.exePath)
		if chosen.name == "Microsoft Edge" && strings.TrimSpace(edgeIDEdit.Text()) != "" {
			if err := saveEdgeExtensionID(edgeIDEdit.Text()); err != nil {
				logBrowserInstall("Invalid Edge extension ID: %v", err)
				setStatus("Cannot register Edge ID: " + err.Error())
				return
			}
			logBrowserInstall("Saved Edge extension ID for native messaging")
		}

		exe, err := os.Executable()
		if err != nil {
			logBrowserInstall("Cannot locate application executable: %v", err)
			setStatus("Cannot locate executable: " + err.Error())
			win.Close()
			return
		}
		extDir := filepath.Join(filepath.Dir(exe), "browser-extension")
		if _, err := os.Stat(extDir); os.IsNotExist(err) {
			logBrowserInstall("Extension folder missing: %s", extDir)
			setStatus(fmt.Sprintf("browser-extension folder not found at: %s", extDir))
			win.Close()
			return
		}

		// Register the Native Messaging host so the extension can sync settings.
		if regErr := registerNMHostOnly(); regErr != nil {
			logBrowserInstall("Native messaging registration failed: %v", regErr)
			setStatus("Warning: could not register native messaging host: " + regErr.Error())
		} else {
			logBrowserInstall("Native messaging host registered")
		}

		// Branded Chrome and Edge intentionally ignore --load-extension.
		// Opening the folder and extensions page enables Edge's supported,
		// user-confirmed unpacked extension installation flow.
		clipErr := copyToClipboard(extDir)
		if clipErr != nil {
			logBrowserInstall("Could not copy extension path to clipboard: %v", clipErr)
		} else {
			logBrowserInstall("Copied extension path to clipboard: %s", extDir)
		}

		extURL := extensionsPageURL(chosen.name)
		if err := exec.Command(chosen.exePath, extURL).Start(); err != nil {
			logBrowserInstall("Could not open %s: %v", extURL, err)
			setStatus(fmt.Sprintf("Could not open %s: %v. Log: %s", chosen.name, err, browserInstallLogPath()))
			win.Close()
			return
		}
		logBrowserInstall("Opened %s", extURL)
		if err := exec.Command("explorer.exe", extDir).Start(); err != nil {
			logBrowserInstall("Could not open extension folder %s: %v", extDir, err)
		} else {
			logBrowserInstall("Opened extension folder: %s", extDir)
		}

		if clipErr == nil {
			setStatus(fmt.Sprintf(
				"%s and extension folder opened. Turn on Developer mode, click \"Load unpacked\", select the opened folder. Log: %s",
				chosen.name, browserInstallLogPath()))
		} else {
			setStatus(fmt.Sprintf(
				"%s and extension folder opened. Turn on Developer mode, click \"Load unpacked\", then select the folder. Log: %s",
				chosen.name, browserInstallLogPath()))
		}
		win.Close()
	})
	win.Add(installBtn)

	cancelBtn := wui.NewButton()
	cancelBtn.SetText("Cancel")
	cancelBtn.SetBounds(214, 156+len(browsers)*32, 192, 32)
	cancelBtn.SetOnClick(func() { win.Close() })
	win.Add(cancelBtn)

	win.ShowModal() //nolint:errcheck
}

// extensionsPageURL returns the internal Extensions-management page URL for
// a given detected browser, so it can be opened directly to the right place
// for the user to enable Developer mode and click "Load unpacked".
func extensionsPageURL(browserName string) string {
	switch browserName {
	case "Microsoft Edge":
		return "edge://extensions/"
	case "Brave":
		return "brave://extensions/"
	case "Vivaldi":
		return "vivaldi://extensions/"
	default:
		return "chrome://extensions/"
	}
}

// RunBrowserUninstall removes the --load-extension flag from all browser
// shortcuts and unregisters the Native Messaging host.
func RunBrowserUninstall(setStatus func(string)) {
	removed := 0
	for _, b := range detectBrowsers() {
		removed += findAndUnpatchBrowserShortcuts(b.exePath)
	}
	removeNativeMessagingHost()
	if removed > 0 {
		setStatus(fmt.Sprintf("Extension removed from %d shortcut(s). Restart your browser.", removed))
	} else {
		setStatus("No patched shortcuts found. NM host unregistered.")
	}
}

// findAndUnpatchBrowserShortcuts removes --load-extension from every .lnk
// that targets browserExe. Returns the number of shortcuts modified.
func findAndUnpatchBrowserShortcuts(browserExe string) int {
	dirs := []struct {
		path    string
		recurse bool
	}{
		{filepath.Join(os.Getenv("USERPROFILE"), "Desktop"), false},
		{`C:\Users\Public\Desktop`, false},
		{filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs`), true},
		{filepath.Join(os.Getenv("APPDATA"), `Microsoft\Internet Explorer\Quick Launch\User Pinned\TaskBar`), false},
	}
	total := 0
	for _, d := range dirs {
		total += scanAndUnpatchDir(d.path, d.recurse, browserExe)
	}
	return total
}

func scanAndUnpatchDir(dir string, recurse bool, browserExe string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	patched := 0
	for _, e := range entries {
		fullPath := filepath.Join(dir, e.Name())
		if e.IsDir() && recurse {
			patched += scanAndUnpatchDir(fullPath, true, browserExe)
		} else if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".lnk") {
			if ok, _ := unpatchShortcut(fullPath, browserExe); ok {
				patched++
			}
		}
	}
	return patched
}

// unpatchShortcut removes any --load-extension=... token from a shortcut's
// arguments. Returns true if the file was modified.
func unpatchShortcut(lnkPath, browserExe string) (bool, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	procCoInitialize.Call(0) //nolint:errcheck
	defer procCoUninitialize.Call()

	clsid := windows.GUID{Data1: 0x00021401, Data2: 0x0000, Data3: 0x0000,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidSL := windows.GUID{Data1: 0x000214F9, Data2: 0x0000, Data3: 0x0000,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidPF := windows.GUID{Data1: 0x0000010B, Data2: 0x0000, Data3: 0x0000,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}

	var psl uintptr
	r, _, _ := procCoCreateInst.Call(
		uintptr(unsafe.Pointer(&clsid)), 0, 1,
		uintptr(unsafe.Pointer(&iidSL)),
		uintptr(unsafe.Pointer(&psl)),
	)
	if r != 0 {
		return false, nil
	}
	defer comRelease(psl)

	var ppf uintptr
	r, _, _ = comVtblN(psl, 0,
		uintptr(unsafe.Pointer(&iidPF)),
		uintptr(unsafe.Pointer(&ppf)),
	)
	if r != 0 {
		return false, nil
	}
	defer comRelease(ppf)

	r, _, _ = comVtblN(ppf, 5, wstrPtr(lnkPath), 0)
	if r != 0 {
		return false, nil
	}

	pathBuf := make([]uint16, 4096)
	comVtblN(psl, 3, uintptr(unsafe.Pointer(&pathBuf[0])), uintptr(len(pathBuf)), 0, 0) //nolint:errcheck
	target := syscall.UTF16ToString(pathBuf)
	if !strings.EqualFold(target, browserExe) {
		return false, nil
	}

	argsBuf := make([]uint16, 4096)
	comVtblN(psl, 10, uintptr(unsafe.Pointer(&argsBuf[0])), uintptr(len(argsBuf))) //nolint:errcheck
	currentArgs := syscall.UTF16ToString(argsBuf)

	if !strings.Contains(strings.ToLower(currentArgs), "--load-extension") {
		return false, nil // nothing to remove
	}

	// Strip --load-extension=... token (handles quoted and unquoted paths)
	newArgs := removeLoadExtensionArg(currentArgs)
	if err := comVtblCall(psl, 11, wstrPtr(newArgs)); err != nil {
		return false, err
	}
	r, _, _ = comVtblN(ppf, 6, 0, 1)
	if r != 0 {
		return false, fmt.Errorf("IPersistFile.Save: 0x%08X", r)
	}
	return true, nil
}

// removeLoadExtensionArg strips --load-extension=... from a command-line string.
func removeLoadExtensionArg(args string) string {
	// Tokenise manually to handle quoted values correctly.
	var result []string
	i := 0
	for i < len(args) {
		for i < len(args) && args[i] == ' ' {
			i++
		}
		if i >= len(args) {
			break
		}
		var token string
		if args[i] == '"' {
			end := strings.Index(args[i+1:], "\"")
			if end == -1 {
				token = args[i:]
				i = len(args)
			} else {
				token = args[i : i+end+2]
				i += end + 2
			}
		} else {
			end := strings.Index(args[i:], " ")
			if end == -1 {
				token = args[i:]
				i = len(args)
			} else {
				token = args[i : i+end]
				i += end
			}
		}
		if !strings.HasPrefix(strings.ToLower(token), "--load-extension") {
			result = append(result, token)
		}
	}
	return strings.Join(result, " ")
}

// ── IShellLink shortcut creation via COM ─────────────────────────────────────
// We call ole32!CoCreateInstance directly so no child process (PowerShell,
// wscript, etc.) is needed — nothing for antivirus software to flag.

// findAndPatchBrowserShortcuts scans Desktop, Start Menu, and Taskbar for .lnk
// files whose target matches browserExe, and appends loadExtArg to each one's
// arguments so the extension loads automatically every time the browser is opened.
// Returns the number of shortcuts actually modified.
func findAndPatchBrowserShortcuts(browserExe, loadExtArg string) int {
	dirs := []struct {
		path    string
		recurse bool
	}{
		{filepath.Join(os.Getenv("USERPROFILE"), "Desktop"), false},
		{`C:\Users\Public\Desktop`, false},
		{filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs`), true},
		{filepath.Join(os.Getenv("APPDATA"), `Microsoft\Internet Explorer\Quick Launch\User Pinned\TaskBar`), false},
	}
	total := 0
	for _, d := range dirs {
		total += scanAndPatchDir(d.path, d.recurse, browserExe, loadExtArg)
	}
	return total
}

func scanAndPatchDir(dir string, recurse bool, browserExe, loadExtArg string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	patched := 0
	for _, e := range entries {
		fullPath := filepath.Join(dir, e.Name())
		if e.IsDir() && recurse {
			patched += scanAndPatchDir(fullPath, true, browserExe, loadExtArg)
		} else if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".lnk") {
			if ok, _ := patchShortcut(fullPath, browserExe, loadExtArg); ok {
				patched++
			}
		}
	}
	return patched
}

// patchShortcut loads an existing .lnk via IShellLink COM, checks if its target
// matches browserExe, and if so appends loadExtArg to its arguments (unless
// already present). Returns true if the file was modified.
func patchShortcut(lnkPath, browserExe, loadExtArg string) (bool, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	procCoInitialize.Call(0) //nolint:errcheck
	defer procCoUninitialize.Call()

	clsid := windows.GUID{Data1: 0x00021401, Data2: 0x0000, Data3: 0x0000,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidSL := windows.GUID{Data1: 0x000214F9, Data2: 0x0000, Data3: 0x0000,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidPF := windows.GUID{Data1: 0x0000010B, Data2: 0x0000, Data3: 0x0000,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}

	var psl uintptr
	r, _, _ := procCoCreateInst.Call(
		uintptr(unsafe.Pointer(&clsid)), 0, 1,
		uintptr(unsafe.Pointer(&iidSL)),
		uintptr(unsafe.Pointer(&psl)),
	)
	if r != 0 {
		return false, fmt.Errorf("CoCreateInstance: 0x%08X", r)
	}
	defer comRelease(psl)

	var ppf uintptr
	r, _, _ = comVtblN(psl, 0,
		uintptr(unsafe.Pointer(&iidPF)),
		uintptr(unsafe.Pointer(&ppf)),
	)
	if r != 0 {
		return false, fmt.Errorf("QI(IPersistFile): 0x%08X", r)
	}
	defer comRelease(ppf)

	// Load the .lnk file (STGM_READ = 0)
	r, _, _ = comVtblN(ppf, 5, wstrPtr(lnkPath), 0)
	if r != 0 {
		return false, nil // unreadable, skip silently
	}

	// GetPath — check if it targets our browser (vtable[3])
	pathBuf := make([]uint16, 4096)
	comVtblN(psl, 3, uintptr(unsafe.Pointer(&pathBuf[0])), uintptr(len(pathBuf)), 0, 0) //nolint:errcheck
	target := syscall.UTF16ToString(pathBuf)
	if !strings.EqualFold(target, browserExe) {
		return false, nil // not our browser
	}

	// GetArguments — check current args (vtable[10])
	argsBuf := make([]uint16, 4096)
	comVtblN(psl, 10, uintptr(unsafe.Pointer(&argsBuf[0])), uintptr(len(argsBuf))) //nolint:errcheck
	currentArgs := syscall.UTF16ToString(argsBuf)

	if strings.Contains(strings.ToLower(currentArgs), "--load-extension") {
		return false, nil // already patched
	}

	// SetArguments — append our flag (vtable[11])
	newArgs := strings.TrimSpace(currentArgs + " " + loadExtArg)
	if err := comVtblCall(psl, 11, wstrPtr(newArgs)); err != nil {
		return false, fmt.Errorf("SetArguments: %w", err)
	}

	// IPersistFile::Save(NULL, TRUE) — save to same file (vtable[6])
	r, _, _ = comVtblN(ppf, 6, 0, 1)
	if r != 0 {
		return false, fmt.Errorf("IPersistFile.Save: 0x%08X", r)
	}
	return true, nil
}

var (
	ole32DLL           = windows.NewLazySystemDLL("ole32.dll")
	procCoInitialize   = ole32DLL.NewProc("CoInitialize")
	procCoCreateInst   = ole32DLL.NewProc("CoCreateInstance")
	procCoUninitialize = ole32DLL.NewProc("CoUninitialize")

	procOpenClipboard    = user32DLL.NewProc("OpenClipboard")
	procCloseClipboard   = user32DLL.NewProc("CloseClipboard")
	procEmptyClipboard   = user32DLL.NewProc("EmptyClipboard")
	procSetClipboardData = user32DLL.NewProc("SetClipboardData")

	kernel32DLL      = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalAlloc  = kernel32DLL.NewProc("GlobalAlloc")
	procGlobalLock   = kernel32DLL.NewProc("GlobalLock")
	procGlobalUnlock = kernel32DLL.NewProc("GlobalUnlock")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

// copyToClipboard places text on the Windows clipboard as CF_UNICODETEXT so
// the user can paste the extension folder path into the browser's "Load
// unpacked" folder picker.
func copyToClipboard(text string) error {
	utf16, err := syscall.UTF16FromString(text)
	if err != nil {
		return err
	}
	size := uintptr(len(utf16)) * unsafe.Sizeof(uint16(0))

	hMem, _, _ := procGlobalAlloc.Call(gmemMoveable, size)
	if hMem == 0 {
		return fmt.Errorf("GlobalAlloc failed")
	}

	ptr, _, _ := procGlobalLock.Call(hMem)
	if ptr == 0 {
		return fmt.Errorf("GlobalLock failed")
	}
	dst := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(utf16))
	copy(dst, utf16)
	procGlobalUnlock.Call(hMem) //nolint:errcheck

	if r, _, _ := procOpenClipboard.Call(0); r == 0 {
		return fmt.Errorf("OpenClipboard failed")
	}
	defer procCloseClipboard.Call()

	procEmptyClipboard.Call() //nolint:errcheck
	if r, _, _ := procSetClipboardData.Call(cfUnicodeText, hMem); r == 0 {
		return fmt.Errorf("SetClipboardData failed")
	}
	return nil
}

// createShortcut writes a .lnk file at lnkPath pointing to targetExe with the
// given command-line arguments and working directory.
func createShortcut(lnkPath, targetExe, arguments, workingDir string) error {
	// COM must be called from a locked OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	procCoInitialize.Call(0) //nolint:errcheck
	defer procCoUninitialize.Call()

	// CLSID_ShellLink  {00021401-0000-0000-C000-000000000046}
	clsid := windows.GUID{Data1: 0x00021401, Data2: 0x0000, Data3: 0x0000,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	// IID_IShellLinkW  {000214F9-0000-0000-C000-000000000046}
	iidSL := windows.GUID{Data1: 0x000214F9, Data2: 0x0000, Data3: 0x0000,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	// IID_IPersistFile {0000010B-0000-0000-C000-000000000046}
	iidPF := windows.GUID{Data1: 0x0000010B, Data2: 0x0000, Data3: 0x0000,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}

	var psl uintptr
	r, _, _ := procCoCreateInst.Call(
		uintptr(unsafe.Pointer(&clsid)),
		0,
		1, // CLSCTX_INPROC_SERVER
		uintptr(unsafe.Pointer(&iidSL)),
		uintptr(unsafe.Pointer(&psl)),
	)
	if r != 0 {
		return fmt.Errorf("CoCreateInstance: 0x%08X", r)
	}
	defer comRelease(psl)

	// IShellLinkW vtable indices:
	// 0 QI  1 AddRef  2 Release  3 GetPath  4 GetIDList  5 SetIDList
	// 6 GetDesc  7 SetDesc  8 GetWorkDir  9 SetWorkDir  10 GetArgs  11 SetArgs
	// 12 GetHotkey  13 SetHotkey  14 GetShowCmd  15 SetShowCmd
	// 16 GetIconLoc  17 SetIconLoc  18 SetRelPath  19 Resolve  20 SetPath
	if err := comVtblCall(psl, 20, wstrPtr(targetExe)); err != nil { // SetPath
		return fmt.Errorf("SetPath: %w", err)
	}
	if err := comVtblCall(psl, 11, wstrPtr(arguments)); err != nil { // SetArguments
		return fmt.Errorf("SetArguments: %w", err)
	}
	if err := comVtblCall(psl, 9, wstrPtr(workingDir)); err != nil { // SetWorkingDirectory
		return fmt.Errorf("SetWorkingDirectory: %w", err)
	}

	// QueryInterface → IPersistFile
	var ppf uintptr
	r, _, _ = comVtblN(psl, 0,
		uintptr(unsafe.Pointer(&iidPF)),
		uintptr(unsafe.Pointer(&ppf)),
	)
	if r != 0 {
		return fmt.Errorf("QueryInterface(IPersistFile): 0x%08X", r)
	}
	defer comRelease(ppf)

	// IPersistFile vtable: 0 QI  1 AddRef  2 Release  3 GetClassID
	// 4 IsDirty  5 Load  6 Save  7 SaveCompleted  8 GetCurFile
	r, _, _ = comVtblN(ppf, 6, wstrPtr(lnkPath), 1) // Save(path, TRUE)
	if r != 0 {
		return fmt.Errorf("IPersistFile.Save: 0x%08X", r)
	}
	return nil
}

func wstrPtr(s string) uintptr {
	p, _ := syscall.UTF16PtrFromString(s)
	return uintptr(unsafe.Pointer(p))
}

func comVtblN(obj uintptr, idx int, args ...uintptr) (uintptr, uintptr, error) {
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	proc := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	all := make([]uintptr, 1+len(args))
	all[0] = obj
	copy(all[1:], args)
	r, r2, err := syscall.SyscallN(proc, all...)
	return r, r2, err
}

func comVtblCall(obj uintptr, idx int, args ...uintptr) error {
	r, _, _ := comVtblN(obj, idx, args...)
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

func comRelease(obj uintptr) {
	comVtblN(obj, 2) //nolint:errcheck
}

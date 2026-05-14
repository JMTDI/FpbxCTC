use anyhow::Result;
use installrs::gui::*;
use installrs::{source, Installer};
use mslnk::ShellLink;
use std::process::Command;
use winreg::enums::{HKEY_LOCAL_MACHINE, KEY_ALL_ACCESS};
use winreg::RegKey;

const INSTALL_DIR: &str = r"C:\Program Files\FpbxCTC";
const UNINSTALL_REG: &str =
    r"Software\Microsoft\Windows\CurrentVersion\Uninstall\FpbxCTC";

pub fn install(i: &mut Installer) -> Result<()> {
    i.process_commandline()?;
    i.set_out_dir(INSTALL_DIR);

    let mut w = InstallerGui::new("FpbxCTC Setup");
    w.welcome(
        "Welcome to FpbxCTC Setup",
        "This wizard will install FpbxCTC (FreePBX Click to Call) on your computer.\n\nClick Install to begin.",
    );
    w.install_page(|i| {
        // 1. Copy the main executable
        i.file(source!("FpbxCTC.exe"), "FpbxCTC.exe")
            .status("Installing FpbxCTC.exe...")
            .install()?;

        // 2. Write the uninstaller
        i.uninstaller("Uninstall FpbxCTC.exe")
            .status("Writing uninstaller...")
            .install()?;

        // 3. Browser extension files
        i.set_status("Installing browser extension...");
        i.file(source!("browser-extension/manifest.json"),    r"browser-extension\manifest.json").install()?;
        i.file(source!("browser-extension/background.js"),    r"browser-extension\background.js").install()?;
        i.file(source!("browser-extension/icons/icon16.png"),  r"browser-extension\icons\icon16.png").install()?;
        i.file(source!("browser-extension/icons/icon32.png"),  r"browser-extension\icons\icon32.png").install()?;
        i.file(source!("browser-extension/icons/icon48.png"),  r"browser-extension\icons\icon48.png").install()?;
        i.file(source!("browser-extension/icons/icon128.png"), r"browser-extension\icons\icon128.png").install()?;
        i.file(source!("browser-extension/popup/popup.html"),  r"browser-extension\popup\popup.html").install()?;
        i.file(source!("browser-extension/popup/popup.css"),   r"browser-extension\popup\popup.css").install()?;
        i.file(source!("browser-extension/popup/popup.js"),    r"browser-extension\popup\popup.js").install()?;

        // 5. Start Menu shortcut
        i.set_status("Creating Start Menu shortcut...");
        let programdata = std::env::var("PROGRAMDATA").unwrap_or_default();
        let shortcut_path = format!(
            r"{}\Microsoft\Windows\Start Menu\Programs\FpbxCTC.lnk",
            programdata
        );
        let exe_path = format!(r"{}\FpbxCTC.exe", INSTALL_DIR);
        let mut lnk = ShellLink::new(&exe_path)?;
        lnk.set_working_dir(Some(INSTALL_DIR.to_string()));
        lnk.set_icon_location(Some(exe_path.clone()));
        lnk.create_lnk(&shortcut_path)?;

        // 6. Add / Remove Programs registry entry
        i.set_status("Registering with Add/Remove Programs...");
        let hklm = RegKey::predef(HKEY_LOCAL_MACHINE);
        let (key, _) = hklm.create_subkey_with_flags(UNINSTALL_REG, KEY_ALL_ACCESS)?;
        key.set_value("DisplayName", &"FpbxCTC")?;
        key.set_value("DisplayVersion", &"1.0.0")?;
        key.set_value("Publisher", &"FpbxCTC")?;
        key.set_value("DisplayIcon", &format!(r"{}\FpbxCTC.exe,0", INSTALL_DIR))?;
        key.set_value("InstallLocation", &INSTALL_DIR)?;
        key.set_value(
            "UninstallString",
            &format!(r"{}\Uninstall FpbxCTC.exe", INSTALL_DIR),
        )?;
        key.set_value("NoModify", &1u32)?;
        key.set_value("NoRepair", &1u32)?;

        // 7. Register tel: protocol + native messaging host (silent)
        i.set_status("Registering tel: protocol and native messaging host...");
        let exe_path = format!(r"{}\FpbxCTC.exe", INSTALL_DIR);
        let _ = Command::new(&exe_path).arg("-register").output();

        Ok(())
    });
    w.finish_page(
        "Setup Complete",
        "FpbxCTC has been installed successfully.\n\nLaunch it from the Start Menu to configure your API settings.",
    );
    w.run(i)?;

    Ok(())
}

pub fn uninstall(i: &mut Installer) -> Result<()> {
    i.process_commandline()?;

    // Remove Start Menu shortcut
    i.set_status("Removing Start Menu shortcut...");
    let programdata = std::env::var("PROGRAMDATA").unwrap_or_default();
    let shortcut_path = format!(
        r"{}\Microsoft\Windows\Start Menu\Programs\FpbxCTC.lnk",
        programdata
    );
    i.remove(&shortcut_path).install()?;

    // Remove Add / Remove Programs entry
    i.set_status("Removing Add/Remove Programs entry...");
    let hklm = RegKey::predef(HKEY_LOCAL_MACHINE);
    // Ignore error if key doesn't exist
    let _ = hklm.delete_subkey_all(UNINSTALL_REG);

    // Unregister browser extension shortcuts and native messaging host
    i.set_status("Unregistering browser extension...");
    let exe_path = format!(r"{}\FpbxCTC.exe", INSTALL_DIR);
    let _ = Command::new(&exe_path).arg("-uninstall-extension").output();
    let _ = Command::new(&exe_path).arg("-unregister").output();

    // Remove the install directory
    i.remove(INSTALL_DIR).install()?;

    Ok(())
}

# Install the app

Download the latest version from the [releases page](https://github.com/AltairCA/ExitLagFree/releases).

| OS | File | Notes |
|----|------|-------|
| Windows | `ExitLagFree-<version>-windows-amd64-setup.exe` | Recommended. Installs to Program Files, adds Start menu and desktop shortcuts, and an uninstaller. |
| Windows | `ExitLagFree-<version>-windows-amd64-portable.zip` | **Extract it first.** The helper needs `wintun.dll` next to it, so running from inside the zip fails. |
| macOS | `ExitLagFree-<version>-macos-universal.zip` | Universal build for Apple Silicon and Intel. Unzip and move to Applications. |
| Linux | `ExitLagFree-<version>-x86_64.AppImage` | `chmod +x` it and run. |
| Linux | `exitlagfree_<version>_amd64.deb` | `sudo apt install ./exitlagfree_*.deb` |

## First launch: install the helper

The app itself never runs as administrator. A small background service, the helper, creates the tunnel and adds routes for you.

1. Open the app and click **Install helper**.
2. Approve the macOS password prompt, the Windows UAC prompt or the Linux polkit prompt.

You only do this once. The helper only accepts commands from the user account that installed it.

## Updates

Install the new version over the old one.

- **Windows installer:** it also updates the helper automatically.
- **Everything else:** when the running helper doesn't match the one shipped with the app, the app shows **Update helper**. Click it and approve the prompt. If you're connected, the tunnel drops for a moment.

## Unsigned builds

The builds aren't code-signed yet, so your OS warns the first time:

- **macOS:** right-click the app, choose **Open**, then **Open** again.
- **Windows:** SmartScreen shows "Windows protected your PC". Click **More info**, then **Run anyway**.

## Uninstall

- **Windows (installer):** uninstall ExitLagFree from **Settings > Apps**. This also removes the helper service.
- **Windows (portable), macOS, Linux:** remove the helper first. From a terminal in the app's folder, run `exitlag-helper uninstall` as administrator or root. On macOS the helper is inside the app at `ExitLagFree.app/Contents/MacOS/exitlag-helper`. Then delete the app.

Your paired nodes and settings live in your user config directory under `ExitLagFree` (for example `~/Library/Application Support/ExitLagFree` on macOS). Device keys are stored in the OS keychain, or in a file only your user can read if no keychain is available.

## Nightly builds

Every push to `master` publishes a **Nightly** pre-release on the [releases page](https://github.com/AltairCA/ExitLagFree/releases/tag/nightly). It may be unstable. Nightly versions change with every build, so the app will offer **Update helper** after each one unless you use the Windows installer.

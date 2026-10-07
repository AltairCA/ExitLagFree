import { useState } from "react";
import { api, errorText } from "../api";

interface Props {
  // Set when a helper is running but differs from the one bundled with the app.
  update?: { running: string; bundled: string; connected: boolean };
  onInstalled(): void;
  notify(kind: "error" | "info", text: string): void;
}

export function HelperBanner({ update, onInstalled, notify }: Props) {
  const [busy, setBusy] = useState(false);

  const install = async () => {
    setBusy(true);
    try {
      await api.InstallHelper();
      notify("info", update ? "Helper updated." : "Helper installed.");
      onInstalled();
    } catch (e) {
      notify("error", errorText(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="banner">
      {update ? (
        <div>
          <strong>Helper update available:</strong> the background service is version {update.running || "unknown"}{" "}
          but this app ships {update.bundled}. Update it to get the latest fixes.
          {update.connected && " This will briefly disconnect your tunnel."}
        </div>
      ) : (
        <div>
          <strong>One-time setup:</strong> ExitLagFree needs a small background service to create the tunnel and
          set routes. You'll be asked for your password once; the app itself never runs as administrator.
        </div>
      )}
      <button className="primary" disabled={busy} onClick={install}>
        {busy ? "Waiting for approval…" : update ? "Update helper" : "Install helper"}
      </button>
    </div>
  );
}

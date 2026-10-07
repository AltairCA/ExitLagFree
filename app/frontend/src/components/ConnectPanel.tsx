import { useState, type MouseEvent } from "react";
import { api, DOCS_URL, errorText, HelperStatus, Node, NodePing, openURL } from "../api";
import { ago, bytes, duration, msLabel, pingClass } from "../format";

const link = (url: string) => (e: MouseEvent) => {
  e.preventDefault();
  openURL(url);
};

interface Props {
  node: Node | null;
  status: HelperStatus;
  helperInstalled: boolean;
  ping?: NodePing;
  onChanged(): void;
  onRemoved(): void;
  notify(kind: "error" | "info", text: string): void;
}

export function ConnectPanel({ node, status, helperInstalled, ping, onChanged, onRemoved, notify }: Props) {
  const [busy, setBusy] = useState(false);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [editing, setEditing] = useState<string | null>(null);

  if (!node) {
    return (
      <section className="card hero">
        <h2>Add your first node</h2>
        <ol className="steps">
          <li>
            On a Linux VPS near the game servers, run:
            <pre>curl -fsSL https://github.com/AltairCA/ExitLagFree/releases/latest/download/install.sh | sudo bash</pre>
          </li>
          <li>Copy the <code>elf://</code> invite link it prints.</li>
          <li>Click <strong>+ Add node</strong> and paste it.</li>
        </ol>
        <p className="muted">
          Server already runs other services, or prefer Docker?{" "}
          <a href={`${DOCS_URL}/node/manual-install`} onClick={link(`${DOCS_URL}/node/manual-install`)}>
            Manual setup
          </a>{" "}
          ·{" "}
          <a href={`${DOCS_URL}/node/docker`} onClick={link(`${DOCS_URL}/node/docker`)}>
            Docker
          </a>{" "}
          ·{" "}
          <a href={DOCS_URL} onClick={link(DOCS_URL)}>
            All docs
          </a>
        </p>
      </section>
    );
  }

  const connectedHere = status.state === "connected" && status.node_id === node.id;
  const connectedElsewhere = status.state === "connected" && status.node_id !== node.id;

  const run = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    try {
      await fn();
      onChanged();
    } catch (e) {
      notify("error", errorText(e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    if (!confirmRemove) {
      setConfirmRemove(true);
      setTimeout(() => setConfirmRemove(false), 4000);
      return;
    }
    setBusy(true);
    try {
      await api.RemoveNode(node.id);
      onRemoved();
    } catch (e) {
      notify("error", errorText(e));
    } finally {
      setBusy(false);
      setConfirmRemove(false);
    }
  };

  const saveName = () => {
    const name = editing?.trim();
    setEditing(null);
    if (name && name !== node.name) run(() => api.RenameNode(node.id, name));
  };

  return (
    <section className={`card hero ${connectedHere ? "is-on" : ""}`}>
      <div className="hero-top">
        <div>
          {editing !== null ? (
            <input
              className="rename"
              autoFocus
              value={editing}
              onChange={(e) => setEditing(e.target.value)}
              onBlur={saveName}
              onKeyDown={(e) => {
                if (e.key === "Enter") saveName();
                if (e.key === "Escape") setEditing(null);
              }}
            />
          ) : (
            <h2 onDoubleClick={() => setEditing(node.name)} title="Double-click to rename">
              {node.name}
            </h2>
          )}
          <div className="muted">
            {node.host} · paired as <em>{node.device_name}</em> ({node.address})
          </div>
        </div>
        <div className="hero-actions">
          <button
            className={confirmRemove ? "danger" : "ghost"}
            disabled={busy}
            onClick={remove}
            title="Unpairs this device from the node; you'll need a new invite to reconnect"
          >
            {confirmRemove ? "Click again to unpair" : "Remove"}
          </button>
          {connectedHere ? (
            <button className="danger big" disabled={busy} onClick={() => run(api.Disconnect)}>
              Disconnect
            </button>
          ) : (
            <button
              className="primary big"
              disabled={busy || !helperInstalled}
              title={helperInstalled ? "" : "Install the helper first"}
              onClick={() => run(() => api.Connect(node.id))}
            >
              {busy ? "Connecting…" : connectedElsewhere ? "Switch here" : "Connect"}
            </button>
          )}
        </div>
      </div>

      <div className="stats">
        <Stat label="Node latency" value={ping && !ping.error ? msLabel(ping.rtt_ms) : ping?.error ? "unreachable" : "…"} cls={pingClass(ping?.rtt_ms)} />
        <Stat label="Packet loss" value={ping && !ping.error ? `${ping.loss_pct.toFixed(0)}%` : "-"} />
        <Stat label="Status" value={connectedHere ? "Connected" : status.state === "error" && status.node_id === node.id ? "Error" : "Idle"} cls={connectedHere ? "good" : ""} />
        <Stat label="Uptime" value={connectedHere ? duration(status.connected_at) : "-"} />
        <Stat label="Handshake" value={connectedHere ? ago(status.last_handshake) : "-"} />
        <Stat label="Routes" value={connectedHere ? String(status.routes) : "-"} />
        <Stat label="Down / Up" value={connectedHere ? `${bytes(status.rx_bytes)} / ${bytes(status.tx_bytes)}` : "-"} />
        <Stat label="Interface" value={connectedHere ? status.interface ?? "-" : "-"} />
      </div>

      {status.state === "error" && status.error && <div className="error-line">{status.error}</div>}
      {connectedHere && status.last_handshake?.startsWith("0001") && (
        <div className="warn-line">
          No handshake yet. Check that UDP {node.endpoint.split(":").pop()} is open on the VPS and in your provider's firewall.
        </div>
      )}
    </section>
  );
}

function Stat({ label, value, cls = "" }: { label: string; value: string; cls?: string }) {
  return (
    <div className="stat">
      <div className="stat-label">{label}</div>
      <div className={`stat-value ${cls}`}>{value}</div>
    </div>
  );
}

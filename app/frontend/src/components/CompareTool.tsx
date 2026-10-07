import { useState } from "react";
import { api, Comparison, errorText, Node } from "../api";
import { msLabel } from "../format";

interface Props {
  node: Node | null;
  notify(kind: "error" | "info", text: string): void;
}

export function CompareTool({ node, notify }: Props) {
  const [target, setTarget] = useState("");
  const [busy, setBusy] = useState(false);
  const [res, setRes] = useState<Comparison | null>(null);

  if (!node) return null;

  const run = async () => {
    setBusy(true);
    setRes(null);
    try {
      setRes(await api.Compare(node.id, target.trim()));
    } catch (e) {
      notify("error", errorText(e));
    } finally {
      setBusy(false);
    }
  };

  const direct = res && !res.direct_error ? res.direct_ms : undefined;
  const via = res && res.via_node_ms > 0 ? res.via_node_ms : undefined;
  const max = Math.max(direct ?? 0, via ?? 0, 1);
  const verdict =
    direct !== undefined && via !== undefined && !res?.direct_via_tunnel
      ? via < direct
        ? `Routing through ${node.name} should save about ${msLabel(direct - via)}.`
        : `Direct is faster by about ${msLabel(via - direct)}; this node won't help for this server.`
      : null;

  return (
    <section className="card">
      <div className="card-head">
        <h3>Direct vs. via node</h3>
        <span className="muted">Enter a game server IP to estimate whether {node.name} lowers your ping.</span>
      </div>
      <div className="compare-row">
        <input
          value={target}
          placeholder="Game server IP, e.g. 155.133.248.34"
          onChange={(e) => setTarget(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && target && run()}
        />
        <button className="primary" disabled={busy || !target} onClick={run}>
          {busy ? "Measuring…" : "Compare"}
        </button>
      </div>

      {res && (
        <div className="compare">
          <Bar
            label={res.direct_via_tunnel ? "Now (through tunnel)" : "Direct from this PC"}
            value={direct}
            max={max}
            error={res.direct_error}
            cls={res.direct_via_tunnel ? "via" : "direct"}
          />
          <Bar
            label={`Via ${node.name} (estimated)`}
            value={via}
            max={max}
            error={res.node_leg_error || res.node_target_error}
            cls="via"
            detail={via !== undefined ? `${msLabel(res.node_leg_ms)} to node + ${msLabel(res.node_to_target_ms)} node to server` : undefined}
          />
          {verdict && <div className="verdict">{verdict}</div>}
          {res.direct_via_tunnel && (
            <div className="muted small">This server is already routed through your tunnel, so the first bar is your real tunnelled latency.</div>
          )}
        </div>
      )}
    </section>
  );
}

function Bar({ label, value, max, error, cls, detail }: { label: string; value?: number; max: number; error?: string; cls: string; detail?: string }) {
  return (
    <div className="bar-row">
      <div className="bar-label">{label}</div>
      <div className="bar-track">
        {value !== undefined && <div className={`bar ${cls}`} style={{ width: `${Math.max(4, (value / max) * 100)}%` }} />}
      </div>
      <div className="bar-value">{value !== undefined ? msLabel(value) : <span className="bad small">{error ?? "-"}</span>}</div>
      {detail && <div className="bar-detail">{detail}</div>}
    </div>
  );
}

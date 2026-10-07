import { Node, NodePing } from "../api";
import { msLabel, pingClass } from "../format";

interface Props {
  nodes: Node[];
  pings: Record<string, NodePing>;
  selected: string | null;
  connectedId?: string;
  onSelect(id: string): void;
  onAdd(): void;
  onRefreshPings(): void;
  version: string;
}

export function Sidebar({ nodes, pings, selected, connectedId, onSelect, onAdd, onRefreshPings, version }: Props) {
  return (
    <aside className="sidebar">
      <div className="brand">
        <span className="logo">⚡</span>
        <div>
          <div className="brand-name">ExitLagFree</div>
          <div className="brand-sub">self-hosted game relay</div>
        </div>
      </div>

      <div className="section-head">
        <span>Your nodes</span>
        <button className="link" onClick={onRefreshPings} title="Re-measure latency">
          refresh
        </button>
      </div>

      <ul className="node-list">
        {nodes.length === 0 && <li className="empty">No nodes yet. Run the installer on your VPS, then add its invite link here.</li>}
        {nodes.map((n) => {
          const p = pings[n.id];
          return (
            <li key={n.id} className={n.id === selected ? "active" : ""} onClick={() => onSelect(n.id)}>
              <div className="node-row">
                <span className={`dot ${n.id === connectedId ? "on" : ""}`} />
                <span className="node-name">{n.name}</span>
                {p && !p.error && <span className={`ping ${pingClass(p.rtt_ms)}`}>{msLabel(p.rtt_ms)}</span>}
                {p?.error && <span className="ping bad" title={p.error}>offline</span>}
              </div>
              <div className="node-host">{n.host}</div>
            </li>
          );
        })}
      </ul>

      <button className="primary wide" onClick={onAdd}>
        + Add node
      </button>
      <div className="version">v{version}</div>
    </aside>
  );
}

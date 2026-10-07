import { useCallback, useEffect, useState } from "react";
import { api, AppState, errorText, NodePing } from "./api";
import { Sidebar } from "./components/Sidebar";
import { HelperBanner } from "./components/HelperBanner";
import { ConnectPanel } from "./components/ConnectPanel";
import { ProfilesPanel } from "./components/ProfilesPanel";
import { CompareTool } from "./components/CompareTool";
import { AddNodeDialog } from "./components/AddNodeDialog";

export default function App() {
  const [state, setState] = useState<AppState | null>(null);
  const [pings, setPings] = useState<Record<string, NodePing>>({});
  const [selected, setSelected] = useState<string | null>(null);
  const [adding, setAdding] = useState(false);
  const [toast, setToast] = useState<{ kind: "error" | "info"; text: string } | null>(null);

  const refresh = useCallback(async () => {
    try {
      const s = await api.GetState();
      setState(s);
      setSelected((cur) => {
        if (cur && s.nodes.some((n) => n.id === cur)) return cur;
        return s.status.node_id || s.last_node || s.nodes[0]?.id || null;
      });
    } catch (e) {
      setToast({ kind: "error", text: errorText(e) });
    }
  }, []);

  const refreshPings = useCallback(async () => {
    try {
      const list = await api.PingNodes();
      setPings(Object.fromEntries(list.map((p) => [p.node_id, p])));
    } catch {
      /* ignore; shown per node */
    }
  }, []);

  useEffect(() => {
    refresh();
    refreshPings();
    const a = setInterval(refresh, 2000);
    const b = setInterval(refreshPings, 10000);
    return () => {
      clearInterval(a);
      clearInterval(b);
    };
  }, [refresh, refreshPings]);

  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(null), 6000);
    return () => clearTimeout(t);
  }, [toast]);

  const notify = (kind: "error" | "info", text: string) => setToast({ kind, text });

  if (!state) {
    return <div className="loading">Loading…</div>;
  }

  const node = state.nodes.find((n) => n.id === selected) ?? null;

  return (
    <div className="app">
      <Sidebar
        nodes={state.nodes}
        pings={pings}
        selected={selected}
        connectedId={state.status.state === "connected" ? state.status.node_id : undefined}
        onSelect={setSelected}
        onAdd={() => setAdding(true)}
        onRefreshPings={refreshPings}
        version={state.version}
      />
      <main className="main">
        {!state.helper_installed && <HelperBanner onInstalled={refresh} notify={notify} />}
        {state.helper_installed && state.helper_outdated && (
          <HelperBanner
            update={{
              running: state.status.version,
              bundled: state.bundled_helper_version ?? "",
              connected: state.status.state === "connected",
            }}
            onInstalled={refresh}
            notify={notify}
          />
        )}
        <ConnectPanel
          node={node}
          status={state.status}
          helperInstalled={state.helper_installed}
          ping={node ? pings[node.id] : undefined}
          onChanged={refresh}
          onRemoved={() => {
            setSelected(null);
            refresh();
          }}
          notify={notify}
        />
        <ProfilesPanel
          profiles={state.profiles}
          selection={state.selection}
          connected={state.status.state === "connected"}
          onSaved={refresh}
          notify={notify}
        />
        <CompareTool node={node} notify={notify} />
      </main>
      {adding && (
        <AddNodeDialog
          onClose={() => setAdding(false)}
          onAdded={(id) => {
            setAdding(false);
            setSelected(id);
            refresh();
            refreshPings();
            notify("info", "Node paired. Its WireGuard key was generated on this device.");
          }}
        />
      )}
      {toast && (
        <div className={`toast ${toast.kind}`} onClick={() => setToast(null)}>
          {toast.text}
        </div>
      )}
    </div>
  );
}

// Typed wrappers over the Go methods Wails binds on window.go.main.App.

export type TunnelState = "disconnected" | "connecting" | "connected" | "error";

export interface HelperStatus {
  version: string;
  state: TunnelState;
  error?: string;
  node_id?: string;
  node_name?: string;
  interface?: string;
  connected_at?: string;
  last_handshake?: string;
  rx_bytes: number;
  tx_bytes: number;
  routes: number;
}

export interface Node {
  id: string;
  name: string;
  host: string;
  api_port: number;
  device_id: string;
  device_name: string;
  address: string;
  endpoint: string;
  added_at: string;
}

export interface CloudRegion {
  id: string; // "aws:us-east-1" | "gcp:asia-northeast1"
  label: string;
}

export interface Profile {
  id: string;
  name: string;
  description: string;
  notes?: string;
  cidrs: string[];
  cloud?: { aws_services?: string[]; regions: CloudRegion[] };
}

export interface Selection {
  profiles: string[];
  regions?: Record<string, string[]>;
  custom?: string[];
}

export interface AppState {
  version: string;
  helper_installed: boolean;
  helper_error?: string;
  helper_outdated: boolean;
  bundled_helper_version?: string;
  status: HelperStatus;
  nodes: Node[];
  selection: Selection;
  profiles: Profile[];
  last_node?: string;
}

export interface NodePing {
  node_id: string;
  rtt_ms: number;
  loss_pct: number;
  error?: string;
}

export interface Comparison {
  target: string;
  direct_ms: number;
  direct_loss: number;
  direct_error?: string;
  direct_via_tunnel: boolean;
  node_leg_ms: number;
  node_leg_error?: string;
  node_to_target_ms: number;
  node_target_loss: number;
  node_target_error?: string;
  via_node_ms: number;
}

interface GoApp {
  GetState(): Promise<AppState>;
  AddNode(link: string): Promise<Node>;
  RemoveNode(id: string): Promise<void>;
  RenameNode(id: string, name: string): Promise<void>;
  PingNodes(): Promise<NodePing[]>;
  SetSelection(sel: Selection): Promise<void>;
  Connect(nodeID: string): Promise<HelperStatus>;
  Disconnect(): Promise<HelperStatus>;
  Compare(nodeID: string, target: string): Promise<Comparison>;
  InstallHelper(): Promise<void>;
  UninstallHelper(): Promise<void>;
}

declare global {
  interface Window {
    go?: { main?: { App?: GoApp } };
    runtime?: { BrowserOpenURL(url: string): void };
  }
}

export const DOCS_URL = "https://altairca.github.io/ExitLagFree";

// Links inside the Wails webview must be handed to the system browser.
export function openURL(url: string) {
  if (window.runtime) window.runtime.BrowserOpenURL(url);
  else window.open(url, "_blank", "noopener");
}

function app(): GoApp {
  const a = window.go?.main?.App;
  if (!a) throw new Error("Wails runtime not available (open the app via `wails dev` or the built binary)");
  return a;
}

export const api: GoApp = new Proxy({} as GoApp, {
  get: (_t, method: keyof GoApp) => (...args: unknown[]) =>
    (app()[method] as (...a: unknown[]) => Promise<unknown>)(...args),
});

export function errorText(e: unknown): string {
  if (typeof e === "string") return e;
  if (e instanceof Error) return e.message;
  return JSON.stringify(e);
}

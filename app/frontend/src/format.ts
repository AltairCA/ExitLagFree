export function bytes(n: number): string {
  if (n < 1024) return `${n} B`;
  const units = ["KiB", "MiB", "GiB", "TiB"];
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(1)} ${units[i]}`;
}

export function ago(iso?: string): string {
  if (!iso || iso.startsWith("0001")) return "never";
  const s = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  return `${Math.floor(s / 3600)}h ago`;
}

export function duration(iso?: string): string {
  if (!iso || iso.startsWith("0001")) return "-";
  const s = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  return h > 0 ? `${h}h ${m}m` : `${m}m ${sec}s`;
}

export function msLabel(v: number): string {
  return `${v.toFixed(v < 10 ? 1 : 0)} ms`;
}

export function pingClass(ms?: number): string {
  if (ms === undefined) return "";
  if (ms < 50) return "good";
  if (ms < 100) return "ok";
  return "bad";
}

import { useEffect, useState } from "react";
import { api, errorText, Profile, Selection } from "../api";

const providerNames: Record<string, string> = { aws: "AWS", gcp: "Google Cloud" };

function cloudProviders(p: Profile): string[] {
  const ids = new Set((p.cloud?.regions ?? []).map((r) => r.id.split(":")[0]));
  return [...ids].map((id) => providerNames[id] ?? id);
}

interface Props {
  profiles: Profile[];
  selection: Selection;
  connected: boolean;
  onSaved(): void;
  notify(kind: "error" | "info", text: string): void;
}

export function ProfilesPanel({ profiles, selection, connected, onSaved, notify }: Props) {
  const [sel, setSel] = useState<Selection>(selection);
  const [custom, setCustom] = useState((selection.custom ?? []).join("\n"));
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (!dirty) {
      setSel(selection);
      setCustom((selection.custom ?? []).join("\n"));
    }
  }, [selection, dirty]);

  const toggle = (id: string) => {
    const on = sel.profiles.includes(id);
    setSel({ ...sel, profiles: on ? sel.profiles.filter((p) => p !== id) : [...sel.profiles, id] });
    setDirty(true);
  };

  const toggleRegion = (profileId: string, region: string) => {
    const cur = sel.regions?.[profileId] ?? [];
    const next = cur.includes(region) ? cur.filter((r) => r !== region) : [...cur, region];
    setSel({ ...sel, regions: { ...(sel.regions ?? {}), [profileId]: next } });
    setDirty(true);
  };

  const save = async () => {
    const entries = custom
      .split(/[\s,]+/)
      .map((s) => s.trim())
      .filter(Boolean);
    try {
      await api.SetSelection({ ...sel, custom: entries });
      setDirty(false);
      onSaved();
      notify("info", connected ? "Saved. Reconnect to apply the new routes." : "Saved.");
    } catch (e) {
      notify("error", errorText(e));
    }
  };

  return (
    <section className="card">
      <div className="card-head">
        <h3>Games</h3>
        <span className="muted">Only traffic to these networks goes through your node; everything else stays direct.</span>
      </div>
      <div className="profiles">
        {profiles.map((p) => {
          const on = sel.profiles.includes(p.id);
          return (
            <div key={p.id} className={`profile ${on ? "on" : ""}`}>
              <label className="profile-head">
                <input type="checkbox" checked={on} onChange={() => toggle(p.id)} />
                <span className="profile-name">{p.name}</span>
                {p.cidrs.length > 0 && <span className="pill">{p.cidrs.length} ranges</span>}
                {cloudProviders(p).map((name) => (
                  <span key={name} className="pill">
                    {name}
                  </span>
                ))}
              </label>
              <div className="profile-desc" title={p.notes}>
                {p.description}
              </div>
              {on && p.cloud && (
                <div className="regions">
                  {p.cloud.regions.map((r) => (
                    <label key={r.id} className="region">
                      <input
                        type="checkbox"
                        checked={(sel.regions?.[p.id] ?? []).includes(r.id)}
                        onChange={() => toggleRegion(p.id, r.id)}
                      />
                      {r.label}
                    </label>
                  ))}
                </div>
              )}
            </div>
          );
        })}
      </div>

      <div className="custom">
        <label className="muted" htmlFor="custom">
          Custom routes (public IPv4 addresses or CIDRs, one per line)
        </label>
        <textarea
          id="custom"
          rows={3}
          placeholder={"e.g. 203.0.113.50\n198.51.100.0/24"}
          value={custom}
          onChange={(e) => {
            setCustom(e.target.value);
            setDirty(true);
          }}
        />
      </div>
      <div className="card-foot">
        <button className="primary" disabled={!dirty} onClick={save}>
          Save selection
        </button>
      </div>
    </section>
  );
}

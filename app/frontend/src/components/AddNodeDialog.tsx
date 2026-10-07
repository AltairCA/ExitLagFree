import { useState } from "react";
import { api, errorText } from "../api";

interface Props {
  onClose(): void;
  onAdded(id: string): void;
}

export function AddNodeDialog({ onClose, onAdded }: Props) {
  const [link, setLink] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    setBusy(true);
    setError(null);
    try {
      const n = await api.AddNode(link.trim());
      onAdded(n.id);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <h3>Add node</h3>
        <p className="muted">
          On your VPS run <code>sudo exitlag-node invite --name "my-pc"</code> and paste the one-time link below. The
          link pins the node's certificate, so nobody can impersonate it.
        </p>
        <textarea
          autoFocus
          rows={3}
          placeholder="elf://203.0.113.10:8443?t=…&fp=…"
          value={link}
          onChange={(e) => setLink(e.target.value)}
        />
        {error && <div className="error-line">{error}</div>}
        <div className="modal-actions">
          <button className="ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="primary" disabled={busy || !link.trim().startsWith("elf://")} onClick={submit}>
            {busy ? "Pairing…" : "Pair"}
          </button>
        </div>
      </div>
    </div>
  );
}

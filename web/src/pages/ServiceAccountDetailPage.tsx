import { useState, type FormEvent } from "react";
import { useParams } from "react-router-dom";

import { useCreateApiKey, useRevokeApiKey } from "../api/hooks/useIdentity";
import { PERMISSIONS, type ApiKeyCreateResponse } from "../api/types";

export function ServiceAccountDetailPage() {
  const { orgId = "", saId = "" } = useParams();
  const createKey = useCreateApiKey(orgId, saId);
  const revokeKey = useRevokeApiKey(orgId, saId);

  const [name, setName] = useState("");
  const [scopes, setScopes] = useState<Set<string>>(new Set());
  const [shownKey, setShownKey] = useState<ApiKeyCreateResponse | null>(null);
  const [knownKeyIds, setKnownKeyIds] = useState<string[]>([]);

  function toggleScope(p: string) {
    setScopes((prev) => {
      const next = new Set(prev);
      if (next.has(p)) next.delete(p);
      else next.add(p);
      return next;
    });
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    const key = await createKey.mutateAsync({ name, scopes: [...scopes] });
    setShownKey(key);
    setKnownKeyIds((prev) => [...prev, key.id]);
    setName("");
    setScopes(new Set());
  }

  return (
    <div>
      <div className="page-header">
        <h1>API keys</h1>
      </div>

      <div className="card">
        <h3>Known keys (created this session)</h3>
        {knownKeyIds.length === 0 && <p className="muted">None yet.</p>}
        {knownKeyIds.map((id) => (
          <div key={id} className="field-row" style={{ alignItems: "center", marginBottom: 6 }}>
            <span className="mono">{id}</span>
            <button className="danger" onClick={() => revokeKey.mutate(id)} disabled={revokeKey.isPending}>
              Revoke
            </button>
          </div>
        ))}
      </div>

      <div className="card" style={{ maxWidth: 480 }}>
        <h3>Create API key</h3>
        <form onSubmit={onSubmit}>
          <label>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} required />
          </label>
          <div>
            <span className="muted">Scopes (leave empty for unrestricted)</span>
            {PERMISSIONS.map((p) => (
              <label key={p} style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
                <input type="checkbox" checked={scopes.has(p)} onChange={() => toggleScope(p)} />
                <span className="mono">{p}</span>
              </label>
            ))}
          </div>
          <button type="submit" disabled={createKey.isPending}>
            Create
          </button>
        </form>
      </div>

      {shownKey && (
        <div className="modal-backdrop" onClick={() => setShownKey(null)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h3>API key created</h3>
            <div className="error-banner">You will not be able to see this key again - store it now.</div>
            <p className="mono" style={{ wordBreak: "break-all", userSelect: "all" }}>
              {shownKey.key}
            </p>
            <button
              className="secondary"
              onClick={() => {
                navigator.clipboard.writeText(shownKey.key);
              }}
            >
              Copy to clipboard
            </button>{" "}
            <button onClick={() => setShownKey(null)}>Close</button>
          </div>
        </div>
      )}
    </div>
  );
}

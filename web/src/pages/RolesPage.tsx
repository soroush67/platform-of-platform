import { useState, type FormEvent } from "react";
import { useParams } from "react-router-dom";

import { useCreateRole, useRoles } from "../api/hooks/useRbac";
import { PERMISSIONS } from "../api/types";

export function RolesPage() {
  const { orgId = "" } = useParams();
  const { data, isLoading } = useRoles(orgId);
  const createRole = useCreateRole(orgId);

  const [name, setName] = useState("");
  const [permissions, setPermissions] = useState<Set<string>>(new Set());

  function togglePermission(p: string) {
    setPermissions((prev) => {
      const next = new Set(prev);
      if (next.has(p)) next.delete(p);
      else next.add(p);
      return next;
    });
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    await createRole.mutateAsync({ name, permissions: [...permissions] });
    setName("");
    setPermissions(new Set());
  }

  return (
    <div>
      <div className="page-header">
        <h1>Roles</h1>
      </div>

      {isLoading && <p className="muted">Loading…</p>}
      {data && (
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Built-in</th>
              <th>Permissions</th>
            </tr>
          </thead>
          <tbody>
            {data.data.map((r) => (
              <tr key={r.id}>
                <td>{r.name}</td>
                <td>{r.organization_id === null ? <span className="badge badge-dim">built-in</span> : "—"}</td>
                <td>
                  {r.permissions.map((p) => (
                    <span key={p} className="badge" style={{ marginRight: 4 }}>
                      {p}
                    </span>
                  ))}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <div className="card" style={{ marginTop: 20, maxWidth: 480 }}>
        <h3>Create custom role</h3>
        <form onSubmit={onSubmit}>
          <label>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} required />
          </label>
          <div>
            <span className="muted">Permissions</span>
            {PERMISSIONS.map((p) => (
              <label key={p} style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
                <input type="checkbox" checked={permissions.has(p)} onChange={() => togglePermission(p)} />
                <span className="mono">{p}</span>
              </label>
            ))}
          </div>
          <button type="submit" disabled={createRole.isPending}>
            Create
          </button>
        </form>
      </div>
    </div>
  );
}

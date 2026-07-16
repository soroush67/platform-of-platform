import { useState, type FormEvent } from "react";
import { useParams } from "react-router-dom";

import { useCreateRoleBinding, useRoleBindings, useRoles } from "../api/hooks/useRbac";

export function RoleBindingsPage() {
  const { orgId = "" } = useParams();
  const { data: bindings, isLoading } = useRoleBindings(orgId);
  const { data: roles } = useRoles(orgId);
  const createBinding = useCreateRoleBinding(orgId);

  const [roleId, setRoleId] = useState("");
  const [subjectType, setSubjectType] = useState("user");
  const [subjectId, setSubjectId] = useState("");
  const [scopeType, setScopeType] = useState("organization");
  const [scopeId, setScopeId] = useState("");
  const [effect, setEffect] = useState("allow");
  const [formError, setFormError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setFormError(null);
    try {
      await createBinding.mutateAsync({
        role_id: roleId,
        subject: { type: subjectType, id: subjectId },
        scope: { type: scopeType, id: scopeType === "organization" ? orgId : scopeId },
        effect,
      });
      setSubjectId("");
      setScopeId("");
    } catch {
      setFormError("Failed to create role binding.");
    }
  }

  return (
    <div>
      <div className="page-header">
        <h1>Role bindings</h1>
      </div>

      {isLoading && <p className="muted">Loading…</p>}
      {bindings && (
        <table>
          <thead>
            <tr>
              <th>Role</th>
              <th>Subject</th>
              <th>Scope</th>
              <th>Effect</th>
            </tr>
          </thead>
          <tbody>
            {bindings.data.map((b) => (
              <tr key={b.id}>
                <td className="mono">{b.role_id}</td>
                <td className="mono">
                  {b.subject_type}:{b.subject_id}
                </td>
                <td className="mono">
                  {b.scope_type}:{b.scope_id}
                </td>
                <td>
                  <span className={`badge ${b.effect === "deny" ? "badge-danger" : "badge-success"}`}>
                    {b.effect}
                  </span>
                </td>
              </tr>
            ))}
            {bindings.data.length === 0 && (
              <tr>
                <td colSpan={4} className="muted">
                  No role bindings yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      )}

      <div className="card" style={{ marginTop: 20, maxWidth: 480 }}>
        <h3>Create role binding</h3>
        {formError && <div className="error-banner">{formError}</div>}
        <form onSubmit={onSubmit}>
          <label>
            Role
            <select value={roleId} onChange={(e) => setRoleId(e.target.value)} required>
              <option value="">— choose —</option>
              {roles?.data.map((r) => (
                <option key={r.id} value={r.id}>
                  {r.name}
                </option>
              ))}
            </select>
          </label>
          <div className="field-row">
            <label>
              Subject type
              <select value={subjectType} onChange={(e) => setSubjectType(e.target.value)}>
                <option value="user">user</option>
                <option value="service_account">service_account</option>
                <option value="team">team</option>
              </select>
            </label>
            <label>
              Subject ID
              <input value={subjectId} onChange={(e) => setSubjectId(e.target.value)} required />
            </label>
          </div>
          <div className="field-row">
            <label>
              Scope type
              <select value={scopeType} onChange={(e) => setScopeType(e.target.value)}>
                <option value="organization">organization</option>
                <option value="project">project</option>
                <option value="workspace">workspace</option>
              </select>
            </label>
            {scopeType !== "organization" && (
              <label>
                Scope ID
                <input value={scopeId} onChange={(e) => setScopeId(e.target.value)} required />
              </label>
            )}
          </div>
          <label>
            Effect
            <select value={effect} onChange={(e) => setEffect(e.target.value)}>
              <option value="allow">allow</option>
              <option value="deny">deny</option>
            </select>
          </label>
          <button type="submit" disabled={createBinding.isPending}>
            Create
          </button>
        </form>
      </div>
    </div>
  );
}

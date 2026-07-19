import { useState, type FormEvent } from "react";
import { useParams } from "react-router-dom";

import { useCreateRoleBinding, useDeleteRoleBinding, useRoleBindings, useRoles } from "../api/hooks/useRbac";
import { useMembers, useOrganization, useProjects, useTeams } from "../api/hooks/useTenancy";
import { useWorkspaces } from "../api/hooks/useWorkspace";

export function RoleBindingsPage() {
  const { orgId = "" } = useParams();
  const { data: bindings, isLoading } = useRoleBindings(orgId);
  const { data: roles } = useRoles(orgId);
  const { data: members } = useMembers(orgId);
  const { data: teams } = useTeams(orgId);
  const { data: org } = useOrganization(orgId);
  const createBinding = useCreateRoleBinding(orgId);
  const deleteBinding = useDeleteRoleBinding(orgId);
  const [confirmingId, setConfirmingId] = useState<string | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  async function onDelete(bindingId: string) {
    setDeleteError(null);
    try {
      await deleteBinding.mutateAsync(bindingId);
      setConfirmingId(null);
    } catch {
      setDeleteError("Failed to delete role binding.");
    }
  }

  const [roleId, setRoleId] = useState("");
  const [subjectType, setSubjectType] = useState("user");
  const [subjectId, setSubjectId] = useState("");
  const [scopeType, setScopeType] = useState("organization");
  const [scopeId, setScopeId] = useState("");
  // scopeProjectId is only used to narrow the Workspace dropdown below
  // to one Project's own workspaces - it isn't submitted itself
  // (scopeId, set from that dropdown's own selection, is what's sent).
  const [scopeProjectId, setScopeProjectId] = useState("");
  const [effect, setEffect] = useState("allow");
  const [formError, setFormError] = useState<string | null>(null);

  const { data: projects } = useProjects(orgId);
  const { data: workspaces } = useWorkspaces(orgId, scopeProjectId);

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
      setScopeProjectId("");
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
      {deleteError && <div className="error-banner">{deleteError}</div>}
      {bindings && (
        <table>
          <thead>
            <tr>
              <th>Role</th>
              <th>Subject</th>
              <th>Scope</th>
              <th>Effect</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {bindings.data.map((b) => {
              const scopeName = b.scope_type === "organization" ? org?.name : b.scope_name;
              return (
                <tr key={b.id}>
                  <td>{b.role_name || <span className="mono">{b.role_id}</span>}</td>
                  <td>
                    {b.subject_name || <span className="mono">{b.subject_id}</span>}{" "}
                    <span className="muted">({b.subject_type})</span>
                  </td>
                  <td>
                    {scopeName || <span className="mono">{b.scope_id}</span>}{" "}
                    <span className="muted">({b.scope_type})</span>
                  </td>
                  <td>
                    <span className={`badge ${b.effect === "deny" ? "badge-danger" : "badge-success"}`}>
                      {b.effect}
                    </span>
                  </td>
                  <td>
                    {confirmingId === b.id ? (
                      <>
                        <button className="danger" onClick={() => onDelete(b.id)} disabled={deleteBinding.isPending}>
                          Confirm delete
                        </button>{" "}
                        <button className="secondary" onClick={() => setConfirmingId(null)}>
                          Cancel
                        </button>
                      </>
                    ) : (
                      <button className="danger" onClick={() => setConfirmingId(b.id)}>
                        Delete
                      </button>
                    )}
                  </td>
                </tr>
              );
            })}
            {bindings.data.length === 0 && (
              <tr>
                <td colSpan={5} className="muted">
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
              <select
                value={subjectType}
                onChange={(e) => {
                  setSubjectType(e.target.value);
                  setSubjectId("");
                }}
              >
                <option value="user">user</option>
                <option value="service_account">service_account</option>
                <option value="team">team</option>
              </select>
            </label>
            {subjectType === "user" && (
              <label>
                User
                <select value={subjectId} onChange={(e) => setSubjectId(e.target.value)} required>
                  <option value="">— choose —</option>
                  {members?.data.map((m) => (
                    <option key={m.user_id} value={m.user_id}>
                      {m.username}
                    </option>
                  ))}
                </select>
              </label>
            )}
            {subjectType === "team" && (
              <label>
                Team (Group)
                <select value={subjectId} onChange={(e) => setSubjectId(e.target.value)} required>
                  <option value="">— choose —</option>
                  {teams?.data.map((t) => (
                    <option key={t.id} value={t.id}>
                      {t.name}
                    </option>
                  ))}
                </select>
              </label>
            )}
            {subjectType === "service_account" && (
              <label>
                Subject ID
                <input value={subjectId} onChange={(e) => setSubjectId(e.target.value)} required />
              </label>
            )}
          </div>
          <div className="field-row">
            <label>
              Scope type
              <select
                value={scopeType}
                onChange={(e) => {
                  setScopeType(e.target.value);
                  setScopeId("");
                  setScopeProjectId("");
                }}
              >
                <option value="organization">organization</option>
                <option value="project">project</option>
                <option value="workspace">workspace</option>
              </select>
            </label>
            {scopeType === "project" && (
              <label>
                Project
                <select value={scopeId} onChange={(e) => setScopeId(e.target.value)} required>
                  <option value="">— choose —</option>
                  {projects?.data.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
                </select>
              </label>
            )}
            {scopeType === "workspace" && (
              <>
                <label>
                  Project
                  <select
                    value={scopeProjectId}
                    onChange={(e) => {
                      setScopeProjectId(e.target.value);
                      setScopeId("");
                    }}
                    required
                  >
                    <option value="">— choose —</option>
                    {projects?.data.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  Workspace
                  <select value={scopeId} onChange={(e) => setScopeId(e.target.value)} required disabled={!scopeProjectId}>
                    <option value="">— choose —</option>
                    {workspaces?.data.map((w) => (
                      <option key={w.id} value={w.id}>
                        {w.name}
                      </option>
                    ))}
                  </select>
                </label>
              </>
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

import { useState, type FormEvent } from "react";
import { useParams } from "react-router-dom";

import { useCreateVariable, useDeleteVariable, useResolveVariable, useVariables } from "../api/hooks/useVariables";
import { useProjects } from "../api/hooks/useTenancy";
import { useWorkspaces } from "../api/hooks/useWorkspace";
import type { Variable } from "../api/types";

function VariableValueCell({
  variable,
  orgId,
  projectId,
  workspaceId,
}: {
  variable: Variable;
  orgId: string;
  projectId: string;
  workspaceId: string;
}) {
  const resolve = useResolveVariable(orgId, projectId, workspaceId);
  const [resolved, setResolved] = useState<string | null>(null);

  if (variable.value !== null) {
    return <span className="mono">{variable.value}</span>;
  }
  if (variable.secret_ref === null) {
    return <span className="muted">•••••• (sensitive)</span>;
  }
  return (
    <span className="copy-value">
      <span className="mono muted">
        vault: {variable.secret_ref.mount_id}/{variable.secret_ref.path}
      </span>
      {variable.scope_type === "workspace" && (
        <>
          <button
            className="secondary"
            disabled={resolve.isPending}
            onClick={async () => {
              const v = await resolve.mutateAsync(variable.key);
              setResolved(v.value);
            }}
          >
            Resolve
          </button>
          {resolved !== null && <span className="mono">{resolved}</span>}
        </>
      )}
    </span>
  );
}

export function VariablesPage() {
  const { orgId = "" } = useParams();
  const [scopeType, setScopeType] = useState("organization");
  const [scopeProjectId, setScopeProjectId] = useState("");
  const [scopeWorkspaceId, setScopeWorkspaceId] = useState("");

  const scopeId =
    scopeType === "organization" ? orgId : scopeType === "project" ? scopeProjectId : scopeWorkspaceId;

  const { data: projects } = useProjects(orgId);
  const { data: workspaces } = useWorkspaces(orgId, scopeProjectId);
  const { data: variables, isLoading } = useVariables(orgId, scopeType, scopeId);
  const createVariable = useCreateVariable(orgId);
  const deleteVariable = useDeleteVariable(orgId);

  const [key, setKey] = useState("");
  const [category, setCategory] = useState("env_var");
  const [sensitivity, setSensitivity] = useState("plain");
  const [mode, setMode] = useState<"value" | "secret_ref">("value");
  const [value, setValue] = useState("");
  const [mountId, setMountId] = useState("");
  const [path, setPath] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setFormError(null);
    if (!scopeId) {
      setFormError("Choose a scope first.");
      return;
    }
    try {
      await createVariable.mutateAsync({
        scope_type: scopeType,
        scope_id: scopeId,
        key,
        category,
        sensitivity,
        value: mode === "value" ? value : undefined,
        secret_ref: mode === "secret_ref" ? { mount_id: mountId, path } : null,
      });
      setKey("");
      setValue("");
      setMountId("");
      setPath("");
    } catch {
      setFormError("Failed to create variable.");
    }
  }

  return (
    <div>
      <div className="page-header">
        <h1>Variables</h1>
      </div>

      <div className="card">
        <div className="field-row">
          <label>
            Scope
            <select value={scopeType} onChange={(e) => setScopeType(e.target.value)}>
              <option value="organization">Organization</option>
              <option value="project">Project</option>
              <option value="workspace">Workspace</option>
            </select>
          </label>
          {(scopeType === "project" || scopeType === "workspace") && (
            <label>
              Project
              <select value={scopeProjectId} onChange={(e) => setScopeProjectId(e.target.value)}>
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
            <label>
              Workspace
              <select value={scopeWorkspaceId} onChange={(e) => setScopeWorkspaceId(e.target.value)}>
                <option value="">— choose —</option>
                {workspaces?.data.map((w) => (
                  <option key={w.id} value={w.id}>
                    {w.name}
                  </option>
                ))}
              </select>
            </label>
          )}
        </div>
      </div>

      {isLoading && <p className="muted">Loading…</p>}
      {variables && (
        <table>
          <thead>
            <tr>
              <th>Key</th>
              <th>Category</th>
              <th>Sensitivity</th>
              <th>Value</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {variables.data.map((v) => (
              <tr key={v.id}>
                <td className="mono">{v.key}</td>
                <td>{v.category}</td>
                <td>
                  {v.sensitivity === "sensitive" ? (
                    <span className="badge badge-warning">sensitive</span>
                  ) : (
                    <span className="badge badge-dim">plain</span>
                  )}
                </td>
                <td>
                  <VariableValueCell
                    variable={v}
                    orgId={orgId}
                    projectId={scopeProjectId}
                    workspaceId={scopeWorkspaceId}
                  />
                </td>
                <td>
                  <button className="danger" onClick={() => deleteVariable.mutate(v.id)}>
                    Delete
                  </button>
                </td>
              </tr>
            ))}
            {variables.data.length === 0 && (
              <tr>
                <td colSpan={5} className="muted">
                  No variables at this scope.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      )}

      <div className="card" style={{ marginTop: 20, maxWidth: 480 }}>
        <h3>Create variable</h3>
        {formError && <div className="error-banner">{formError}</div>}
        <form onSubmit={onSubmit}>
          <label>
            Key
            <input value={key} onChange={(e) => setKey(e.target.value)} required />
          </label>
          <div className="field-row">
            <label>
              Category
              <select value={category} onChange={(e) => setCategory(e.target.value)}>
                <option value="env_var">env_var</option>
                <option value="engine_var">engine_var</option>
                <option value="file_template">file_template</option>
              </select>
            </label>
            <label>
              Sensitivity
              <select value={sensitivity} onChange={(e) => setSensitivity(e.target.value)}>
                <option value="plain">plain</option>
                <option value="sensitive">sensitive</option>
              </select>
            </label>
          </div>
          <div className="field-row">
            <label>
              <input type="radio" checked={mode === "value"} onChange={() => setMode("value")} /> Plain value
            </label>
            <label>
              <input type="radio" checked={mode === "secret_ref"} onChange={() => setMode("secret_ref")} /> Vault
              reference
            </label>
          </div>
          {mode === "value" ? (
            <label>
              Value
              <input value={value} onChange={(e) => setValue(e.target.value)} required />
            </label>
          ) : (
            <>
              <label>
                Secret mount ID
                <input value={mountId} onChange={(e) => setMountId(e.target.value)} required />
              </label>
              <label>
                Path
                <input
                  value={path}
                  onChange={(e) => setPath(e.target.value)}
                  placeholder="secret/data/database/prod"
                  required
                />
              </label>
            </>
          )}
          <button type="submit" disabled={createVariable.isPending}>
            Create
          </button>
        </form>
      </div>
    </div>
  );
}

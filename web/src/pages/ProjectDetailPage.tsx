import { useState, type FormEvent } from "react";
import { Link, useParams } from "react-router-dom";

import { useProjectComposeFiles } from "../api/hooks/useFleet";
import { useProject } from "../api/hooks/useTenancy";
import { useCreateWorkspace, useWorkspaces } from "../api/hooks/useWorkspace";
import { EXECUTION_ENGINES } from "../api/types";

export function ProjectDetailPage() {
  const { orgId = "", projectId = "" } = useParams();
  const { data: project } = useProject(orgId, projectId);
  const { data: workspaces } = useWorkspaces(orgId, projectId);
  const { data: composeFiles } = useProjectComposeFiles(orgId, projectId);
  const createWs = useCreateWorkspace(orgId, projectId);

  const [wsName, setWsName] = useState("");
  const [wsEngine, setWsEngine] = useState("terraform");

  async function onCreateWs(e: FormEvent) {
    e.preventDefault();
    await createWs.mutateAsync({ name: wsName, execution_engine: wsEngine, environment_id: null });
    setWsName("");
  }

  return (
    <div>
      <div className="page-header">
        <h1>{project?.name ?? "Project"}</h1>
      </div>

      <div className="section-title">Compose files</div>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Global</th>
          </tr>
        </thead>
        <tbody>
          {composeFiles?.data.map((c) => (
            <tr key={c.id}>
              <td>
                <Link to={`/orgs/${orgId}/compose-files/${c.id}`}>{c.name}</Link>
              </td>
              <td>{c.is_global && <span className="badge">global</span>}</td>
            </tr>
          ))}
          {composeFiles?.data.length === 0 && (
            <tr>
              <td colSpan={2} className="muted">
                No compose files linked to this project yet - link one from its own detail page.
              </td>
            </tr>
          )}
        </tbody>
      </table>

      <div className="section-title">Workspaces</div>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Engine</th>
            <th>Locked</th>
          </tr>
        </thead>
        <tbody>
          {workspaces?.data.map((ws) => (
            <tr key={ws.id}>
              <td>
                <Link to={`/orgs/${orgId}/projects/${projectId}/workspaces/${ws.id}`}>{ws.name}</Link>
              </td>
              <td className="mono">{ws.execution_engine}</td>
              <td>{ws.locked ? <span className="badge badge-warning">locked</span> : "—"}</td>
            </tr>
          ))}
          {workspaces?.data.length === 0 && (
            <tr>
              <td colSpan={3} className="muted">
                No workspaces yet.
              </td>
            </tr>
          )}
        </tbody>
      </table>
      <div className="card" style={{ marginTop: 12, maxWidth: 480 }}>
        <h3>Create workspace</h3>
        <form onSubmit={onCreateWs}>
          <label>
            Name
            <input value={wsName} onChange={(e) => setWsName(e.target.value)} required />
          </label>
          <label>
            Execution engine
            <select value={wsEngine} onChange={(e) => setWsEngine(e.target.value)}>
              {EXECUTION_ENGINES.map((eng) => (
                <option key={eng} value={eng}>
                  {eng}
                </option>
              ))}
            </select>
          </label>
          <button type="submit" disabled={createWs.isPending}>
            Create
          </button>
        </form>
      </div>
    </div>
  );
}

import { useState, type FormEvent } from "react";
import { Link, useParams } from "react-router-dom";

import { useProject } from "../api/hooks/useTenancy";
import { useCreateEnvironment, useCreateWorkspace, useEnvironments, useWorkspaces } from "../api/hooks/useWorkspace";
import { EXECUTION_ENGINES } from "../api/types";

export function ProjectDetailPage() {
  const { orgId = "", projectId = "" } = useParams();
  const { data: project } = useProject(orgId, projectId);
  const { data: environments } = useEnvironments(orgId, projectId);
  const { data: workspaces } = useWorkspaces(orgId, projectId);
  const createEnv = useCreateEnvironment(orgId, projectId);
  const createWs = useCreateWorkspace(orgId, projectId);

  const [envName, setEnvName] = useState("");
  const [envRank, setEnvRank] = useState(0);
  const [envApproval, setEnvApproval] = useState(false);

  const [wsName, setWsName] = useState("");
  const [wsEngine, setWsEngine] = useState("compose");
  const [wsEnvId, setWsEnvId] = useState("");

  async function onCreateEnv(e: FormEvent) {
    e.preventDefault();
    await createEnv.mutateAsync({ name: envName, promotion_rank: envRank, requires_approval: envApproval });
    setEnvName("");
    setEnvRank(0);
    setEnvApproval(false);
  }

  async function onCreateWs(e: FormEvent) {
    e.preventDefault();
    await createWs.mutateAsync({ name: wsName, execution_engine: wsEngine, environment_id: wsEnvId || null });
    setWsName("");
  }

  return (
    <div>
      <div className="page-header">
        <h1>{project?.name ?? "Project"}</h1>
      </div>

      <div className="section-title">Environments</div>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Promotion rank</th>
            <th>Requires approval</th>
          </tr>
        </thead>
        <tbody>
          {environments?.data.map((env) => (
            <tr key={env.id}>
              <td>
                <Link to={`/orgs/${orgId}/projects/${projectId}/environments/${env.id}`}>{env.name}</Link>
              </td>
              <td>{env.promotion_rank}</td>
              <td>{env.requires_approval ? "Yes" : "No"}</td>
            </tr>
          ))}
          {environments?.data.length === 0 && (
            <tr>
              <td colSpan={3} className="muted">
                No environments yet.
              </td>
            </tr>
          )}
        </tbody>
      </table>
      <div className="card" style={{ marginTop: 12, maxWidth: 480 }}>
        <h3>Create environment</h3>
        <form onSubmit={onCreateEnv}>
          <label>
            Name
            <input value={envName} onChange={(e) => setEnvName(e.target.value)} required />
          </label>
          <div className="field-row">
            <label>
              Promotion rank
              <input
                type="number"
                value={envRank}
                onChange={(e) => setEnvRank(Number(e.target.value))}
              />
            </label>
            <label>
              Requires approval
              <input
                type="checkbox"
                checked={envApproval}
                onChange={(e) => setEnvApproval(e.target.checked)}
                style={{ width: 18, height: 18, alignSelf: "flex-start" }}
              />
            </label>
          </div>
          <button type="submit" disabled={createEnv.isPending}>
            Create
          </button>
        </form>
      </div>

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
                  {eng !== "compose" && eng !== "terraform" ? " (not yet implemented)" : ""}
                </option>
              ))}
            </select>
          </label>
          <label>
            Environment (optional)
            <select value={wsEnvId} onChange={(e) => setWsEnvId(e.target.value)}>
              <option value="">— none —</option>
              {environments?.data.map((env) => (
                <option key={env.id} value={env.id}>
                  {env.name}
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

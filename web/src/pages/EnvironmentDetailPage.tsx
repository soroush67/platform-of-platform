import { useParams } from "react-router-dom";

import { useEnvironment } from "../api/hooks/useWorkspace";

export function EnvironmentDetailPage() {
  const { orgId = "", projectId = "", envId = "" } = useParams();
  const { data: env, isLoading } = useEnvironment(orgId, projectId, envId);

  if (isLoading) return <p className="muted">Loading…</p>;
  if (!env) return null;

  return (
    <div>
      <div className="page-header">
        <h1>{env.name}</h1>
      </div>
      <div className="card">
        <p>
          <span className="muted">Promotion rank:</span> {env.promotion_rank}
        </p>
        <p>
          <span className="muted">Requires approval:</span> {env.requires_approval ? "Yes" : "No"}
        </p>
        <p>
          <span className="muted">Created:</span> {new Date(env.created_at).toLocaleString()}
        </p>
      </div>
    </div>
  );
}

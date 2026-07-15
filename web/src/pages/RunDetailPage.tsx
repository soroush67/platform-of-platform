import { useParams } from "react-router-dom";

import { useCancelRun, useRun } from "../api/hooks/useExecution";
import { isTerminalRunStatus } from "../api/types";

export function RunDetailPage() {
  const { orgId = "", projectId = "", workspaceId = "", runId = "" } = useParams();
  const { data: run, isLoading } = useRun(orgId, projectId, workspaceId, runId);
  const cancelRun = useCancelRun(orgId, projectId, workspaceId, runId);

  if (isLoading) return <p className="muted">Loading…</p>;
  if (!run) return null;

  const terminal = isTerminalRunStatus(run.status);

  return (
    <div>
      <div className="page-header">
        <h1>Run {run.id.slice(0, 8)}</h1>
        {!terminal && (
          <button className="danger" onClick={() => cancelRun.mutate()} disabled={cancelRun.isPending}>
            Cancel run
          </button>
        )}
      </div>

      <div className="card">
        <p>
          <span className="muted">Status:</span>{" "}
          <span className="badge">{run.status}</span> {!terminal && <span className="muted">(auto-updating…)</span>}
        </p>
        <p>
          <span className="muted">Trigger:</span> <span className="mono">{run.trigger}</span>
        </p>
        <p>
          <span className="muted">Triggered by:</span> <span className="mono">{run.triggered_by}</span>
        </p>
        <p>
          <span className="muted">Created:</span> {new Date(run.created_at).toLocaleString()}
        </p>
        {run.finished_at && (
          <p>
            <span className="muted">Finished:</span> {new Date(run.finished_at).toLocaleString()}
          </p>
        )}
      </div>

      {run.apply_output_ref && (
        <div className="card">
          <h3>Output</h3>
          <pre className="mono" style={{ whiteSpace: "pre-wrap", maxHeight: 400, overflowY: "auto" }}>
            {run.apply_output_ref}
          </pre>
        </div>
      )}
    </div>
  );
}

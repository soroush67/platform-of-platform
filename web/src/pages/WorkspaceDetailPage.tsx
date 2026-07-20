import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import { useTriggerRun, useRuns } from "../api/hooks/useExecution";
import { useDeleteWorkspace, useWorkspace } from "../api/hooks/useWorkspace";
import type { RunStatus } from "../api/types";

function statusBadgeClass(status: RunStatus): string {
  if (status === "applied") return "badge-success";
  if (status === "failed" || status === "errored") return "badge-danger";
  if (status === "canceled") return "badge-dim";
  return "badge-warning";
}

export function WorkspaceDetailPage() {
  const { orgId = "", projectId = "", workspaceId = "" } = useParams();
  const navigate = useNavigate();
  const { data: workspace } = useWorkspace(orgId, projectId, workspaceId);
  const { data: runs, isLoading } = useRuns(orgId, projectId, workspaceId);
  const triggerRun = useTriggerRun(orgId, projectId, workspaceId);
  const deleteWorkspace = useDeleteWorkspace(orgId, projectId);

  const [error, setError] = useState<string | null>(null);
  // confirmingDelete - same two-step confirm pattern used for every
  // other destructive action in this app (ComposeFileDetailPage's
  // operation trigger, ProjectListPage's bulk delete).
  const [confirmingDelete, setConfirmingDelete] = useState(false);

  async function onTrigger() {
    setError(null);
    try {
      const idempotencyKey = crypto.randomUUID();
      await triggerRun.mutateAsync(idempotencyKey);
    } catch {
      setError("Failed to trigger run - the workspace may be locked or archived.");
    }
  }

  async function onDelete() {
    if (!confirmingDelete) {
      setConfirmingDelete(true);
      return;
    }
    await deleteWorkspace.mutateAsync(workspaceId);
    navigate(`/orgs/${orgId}/projects/${projectId}`);
  }

  return (
    <div>
      <div className="page-header">
        <h1>{workspace?.name ?? "Workspace"}</h1>
        <button onClick={onTrigger} disabled={triggerRun.isPending || workspace?.locked}>
          {triggerRun.isPending ? "Triggering…" : "▶ Trigger run"}
        </button>{" "}
        {confirmingDelete ? (
          <>
            <span className="muted">Permanently delete this workspace and its run history?</span>{" "}
            <button className="danger" onClick={onDelete} disabled={deleteWorkspace.isPending}>
              {deleteWorkspace.isPending ? "Deleting…" : "Confirm delete"}
            </button>{" "}
            <button className="secondary" onClick={() => setConfirmingDelete(false)}>
              Cancel
            </button>
          </>
        ) : (
          <button className="danger" onClick={onDelete}>
            Delete
          </button>
        )}
      </div>
      {workspace?.locked && <div className="error-banner">This workspace is locked - a run is already in progress.</div>}
      {error && <div className="error-banner">{error}</div>}

      <div className="card">
        <p>
          <span className="muted">Execution engine:</span> <span className="mono">{workspace?.execution_engine}</span>
        </p>
      </div>

      <div className="section-title">Runs</div>
      {isLoading && <p className="muted">Loading…</p>}
      <table>
        <thead>
          <tr>
            <th>Status</th>
            <th>Trigger</th>
            <th>Triggered by</th>
            <th>Created</th>
          </tr>
        </thead>
        <tbody>
          {runs?.data.map((run) => (
            <tr key={run.id}>
              <td>
                <Link to={`/orgs/${orgId}/projects/${projectId}/workspaces/${workspaceId}/runs/${run.id}`}>
                  <span className={`badge ${statusBadgeClass(run.status)}`}>{run.status}</span>
                </Link>
              </td>
              <td className="mono">{run.trigger}</td>
              <td className="mono">{run.triggered_by}</td>
              <td className="muted">{new Date(run.created_at).toLocaleString()}</td>
            </tr>
          ))}
          {runs?.data.length === 0 && (
            <tr>
              <td colSpan={4} className="muted">
                No runs yet.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

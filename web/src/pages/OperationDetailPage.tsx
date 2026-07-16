import { useParams } from "react-router-dom";

import { useOperation, useOperationLogStream } from "../api/hooks/useFleet";
import { isTerminalOperationStatus } from "../api/types";

function operationStatusBadgeClass(status: string): string {
  if (status === "success") return "badge-success";
  if (status === "failed") return "badge-danger";
  if (status === "running") return "badge-warning";
  return "badge-dim";
}

export function OperationDetailPage() {
  const { orgId = "", operationId = "" } = useParams();
  const { data: operation, isLoading } = useOperation(orgId, operationId);

  const terminal = operation ? isTerminalOperationStatus(operation.status) : false;
  const stream = useOperationLogStream(orgId, operationId, !!operation && !terminal);

  if (isLoading) return <p className="muted">Loading…</p>;
  if (!operation) return null;

  const liveOutput = stream.lines.join("\n");
  const output = terminal ? (operation.output ?? "") : liveOutput;

  return (
    <div>
      <div className="page-header">
        <h1>Operation {operation.id.slice(0, 8)}</h1>
        <span className={`badge ${operationStatusBadgeClass(operation.status)}`}>{operation.status}</span>
      </div>

      <div className="card">
        <p>
          <span className="muted">Type:</span> <span className="mono">{operation.operation_type}</span>
        </p>
        <p>
          <span className="muted">Triggered by:</span> <span className="mono">{operation.triggered_by}</span>
        </p>
        <p>
          <span className="muted">Created:</span> {new Date(operation.created_at).toLocaleString()}
        </p>
        {operation.started_at && (
          <p>
            <span className="muted">Started:</span> {new Date(operation.started_at).toLocaleString()}
          </p>
        )}
        {operation.finished_at && (
          <p>
            <span className="muted">Finished:</span> {new Date(operation.finished_at).toLocaleString()}
          </p>
        )}
        {operation.exit_code !== undefined && (
          <p>
            <span className="muted">Exit code:</span> <span className="mono">{operation.exit_code}</span>
          </p>
        )}
      </div>

      <div className="card">
        <h3>Output {!terminal && <span className="muted">(live)</span>}</h3>
        {stream.error && <div className="error-banner">{stream.error}</div>}
        <pre className="mono" style={{ whiteSpace: "pre-wrap", maxHeight: 500, overflowY: "auto" }}>
          {output || (terminal ? "(no output)" : "waiting for output…")}
        </pre>
      </div>
    </div>
  );
}

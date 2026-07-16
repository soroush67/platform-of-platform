import { Link, useParams } from "react-router-dom";

import { useComposeFiles, useMachines, useOperations } from "../api/hooks/useFleet";

function operationStatusBadgeClass(status: string): string {
  if (status === "success") return "badge-success";
  if (status === "failed") return "badge-danger";
  if (status === "running") return "badge-warning";
  return "badge-dim";
}

export function OperationsPage() {
  const { orgId = "" } = useParams();
  const { data: operations, isLoading } = useOperations(orgId);
  const { data: composeFiles } = useComposeFiles(orgId);
  const { data: machines } = useMachines(orgId, true);

  return (
    <div>
      <div className="page-header">
        <h1>Operations</h1>
      </div>

      {isLoading && <p className="muted">Loading…</p>}
      <table>
        <thead>
          <tr>
            <th>Status</th>
            <th>Operation</th>
            <th>Compose file</th>
            <th>Machine</th>
            <th>Triggered by</th>
            <th>Created</th>
          </tr>
        </thead>
        <tbody>
          {operations?.data.map((op) => (
            <tr key={op.id}>
              <td>
                <Link to={`/orgs/${orgId}/operations/${op.id}`}>
                  <span className={`badge ${operationStatusBadgeClass(op.status)}`}>{op.status}</span>
                </Link>
              </td>
              <td className="mono">{op.operation_type}</td>
              <td className="mono">
                {composeFiles?.data.find((c) => c.id === op.compose_file_id)?.name ?? op.compose_file_id}
              </td>
              <td className="mono">{machines?.data.find((m) => m.id === op.machine_id)?.name ?? op.machine_id}</td>
              <td className="mono">{op.triggered_by}</td>
              <td className="muted">{new Date(op.created_at).toLocaleString()}</td>
            </tr>
          ))}
          {operations?.data.length === 0 && (
            <tr>
              <td colSpan={6} className="muted">
                No operations yet.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

import { useParams } from "react-router-dom";

import { useAuditLog } from "../api/hooks/useAudit";

export function AuditLogPage() {
  const { orgId = "" } = useParams();
  const { data, isLoading, fetchNextPage, hasNextPage, isFetchingNextPage } = useAuditLog(orgId);

  const entries = data?.pages.flatMap((p) => p.data) ?? [];

  return (
    <div>
      <div className="page-header">
        <h1>Audit log</h1>
      </div>

      {isLoading && <p className="muted">Loading…</p>}
      <table>
        <thead>
          <tr>
            <th>Actor</th>
            <th>Action</th>
            <th>Target</th>
            <th>When</th>
          </tr>
        </thead>
        <tbody>
          {entries.map((e) => (
            <tr key={e.id}>
              <td className="mono">{e.actor}</td>
              <td>{e.action}</td>
              <td className="mono">
                {e.target_type}:{e.target_id}
              </td>
              <td className="muted">{new Date(e.created_at).toLocaleString()}</td>
            </tr>
          ))}
          {entries.length === 0 && !isLoading && (
            <tr>
              <td colSpan={4} className="muted">
                No audit entries yet.
              </td>
            </tr>
          )}
        </tbody>
      </table>

      {hasNextPage && (
        <button className="secondary" style={{ marginTop: 12 }} onClick={() => fetchNextPage()} disabled={isFetchingNextPage}>
          {isFetchingNextPage ? "Loading…" : "Load more"}
        </button>
      )}
    </div>
  );
}

import { Link, useParams } from "react-router-dom";

import { useOrganization } from "../api/hooks/useTenancy";

export function OrgOverviewPage() {
  const { orgId = "" } = useParams();
  const { data: org, isLoading } = useOrganization(orgId);

  if (isLoading) return <p className="muted">Loading…</p>;
  if (!org) return null;

  return (
    <div>
      <div className="page-header">
        <h1>{org.name}</h1>
        <span className={`badge ${org.status === "archived" ? "badge-warning" : "badge-success"}`}>
          {org.status}
        </span>
      </div>
      <div className="card">
        <p>
          <span className="muted">Slug:</span> <span className="mono">{org.slug}</span>
        </p>
        <p>
          <span className="muted">Created:</span> {new Date(org.created_at).toLocaleString()}
        </p>
      </div>
      <div className="card">
        <h3>Quick links</h3>
        <ul>
          <li>
            <Link to={`/orgs/${orgId}/projects`}>Projects</Link>
          </li>
          <li>
            <Link to={`/orgs/${orgId}/members`}>Members</Link>
          </li>
          <li>
            <Link to={`/orgs/${orgId}/audit-log`}>Audit log</Link>
          </li>
        </ul>
      </div>
    </div>
  );
}

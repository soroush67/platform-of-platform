import { useEffect } from "react";
import { useNavigate } from "react-router-dom";

import { useOrganizations } from "../api/hooks/useTenancy";
import { useAuth } from "../auth/AuthContext";

// OrgListPage is only ever actually seen by a non-platform-admin user
// who belongs to zero or 2+ organizations - every other case redirects
// straight past it:
// - A platform admin never stops here at all, straight to /platform-admin
//   (organization creation/management lives there now, not as a gate in
//   front of the app).
// - A regular user in exactly one organization goes straight into it -
//   no "pick your one option" screen.
export function OrgListPage() {
  const { data, isLoading, error } = useOrganizations();
  const { user } = useAuth();
  const navigate = useNavigate();

  useEffect(() => {
    if (user?.is_platform_admin) {
      navigate("/platform-admin", { replace: true });
      return;
    }
    if (data && data.data.length === 1) {
      navigate(`/orgs/${data.data[0].id}`, { replace: true });
    }
  }, [data, user, navigate]);

  return (
    <div className="login-shell" style={{ flexDirection: "column", gap: 24 }}>
      <div className="card" style={{ width: 480 }}>
        <h1>Your organizations</h1>
        {isLoading && <p className="muted">Loading…</p>}
        {error && <div className="error-banner">Failed to load organizations.</div>}
        {data && data.data.length === 0 && (
          <p className="muted">
            You aren't a member of any organization yet - ask a platform admin to add you to one.
          </p>
        )}
        {data && data.data.length > 1 && (
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Slug</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {data.data.map((org) => (
                <tr key={org.id} style={{ cursor: "pointer" }} onClick={() => navigate(`/orgs/${org.id}`)}>
                  <td>{org.name}</td>
                  <td className="mono">{org.slug}</td>
                  <td>
                    <span className={`badge ${org.status === "archived" ? "badge-warning" : "badge-success"}`}>
                      {org.status}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}

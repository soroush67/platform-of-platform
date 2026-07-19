import { useEffect } from "react";
import { useNavigate } from "react-router-dom";

import { useOrganizations } from "../api/hooks/useTenancy";

// OrgListPage is never actually shown as a picker screen - it's a pure
// redirect: login always lands straight inside a real organization's
// own panel, never on an intermediate "choose one" page. Switching
// between organizations happens via OrgLayout's own sidebar dropdown,
// once already inside - that dropdown already covers what a picker
// page would, so this component's only real job is "which org do I
// send a freshly-logged-in user into." Prefers an active org over an
// archived one if the user has both; falls back to the first one at
// all only if every membership happens to be archived. The only case
// with genuinely nothing to redirect to is zero memberships - shown a
// plain message, not a page to navigate from.
export function OrgListPage() {
  const { data, isLoading, error } = useOrganizations();
  const navigate = useNavigate();

  useEffect(() => {
    if (!data || data.data.length === 0) return;
    const target = data.data.find((o) => o.status === "active") ?? data.data[0];
    navigate(`/orgs/${target.id}`, { replace: true });
  }, [data, navigate]);

  if (isLoading || (data && data.data.length > 0)) {
    return null;
  }

  return (
    <div className="login-shell" style={{ flexDirection: "column", gap: 24 }}>
      <div className="card" style={{ width: 480 }}>
        <h1>Your organizations</h1>
        {error && <div className="error-banner">Failed to load organizations.</div>}
        {data && data.data.length === 0 && (
          <p className="muted">You aren't a member of any organization yet - ask an admin to add you to one.</p>
        )}
      </div>
    </div>
  );
}

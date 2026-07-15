import { useState } from "react";
import { useParams } from "react-router-dom";

import { useArchiveOrganization, useOrganization } from "../api/hooks/useTenancy";

export function OrgSettingsPage() {
  const { orgId = "" } = useParams();
  const { data: org } = useOrganization(orgId);
  const archiveOrg = useArchiveOrganization(orgId);
  const [error, setError] = useState<string | null>(null);
  const [confirming, setConfirming] = useState(false);

  async function onArchive() {
    setError(null);
    try {
      await archiveOrg.mutateAsync();
      setConfirming(false);
    } catch {
      // A 403 here just means the caller isn't the org Owner - the
      // client doesn't pre-check locally (no "am I the owner" endpoint
      // to ask), it just surfaces the real backend rejection.
      setError("Failed to archive - this action requires the organization:delete permission (Owner only).");
    }
  }

  return (
    <div>
      <div className="page-header">
        <h1>Settings</h1>
      </div>

      <div className="card">
        <h3>Danger zone</h3>
        {error && <div className="error-banner">{error}</div>}
        {org?.status === "archived" ? (
          <p className="muted">This organization is already archived.</p>
        ) : confirming ? (
          <div>
            <p>Are you sure? This soft-deletes the organization (Owner only).</p>
            <button className="danger" onClick={onArchive} disabled={archiveOrg.isPending}>
              Yes, archive this organization
            </button>{" "}
            <button className="secondary" onClick={() => setConfirming(false)}>
              Cancel
            </button>
          </div>
        ) : (
          <button className="danger" onClick={() => setConfirming(true)}>
            Archive organization
          </button>
        )}
      </div>
    </div>
  );
}

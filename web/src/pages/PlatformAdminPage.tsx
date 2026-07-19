import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";

import {
  useArchiveOrganizationById,
  useCreateOrganization,
  useDeleteOrganization,
  useOrganizations,
} from "../api/hooks/useTenancy";
import type { Organization } from "../api/types";

// Confirming a row's own destructive action - archive is a plain
// confirm/cancel (reversible, Purge Reaper's own 30-day grace window
// still applies), delete needs the org's name/slug actually typed in
// (irreversible - Purge runs immediately, no grace window at all).
type Confirming = { orgId: string; action: "archive" } | { orgId: string; action: "delete"; typed: string };

// PlatformAdminPage is a normal nav item inside the panel (OrgLayout's
// own sidebar, gated to platform admins - see OrgLayout.tsx), not a
// screen shown instead of or before entering the app. Organization
// create/archive/delete all live here now, reachable only once you're
// already inside the panel working normally.
export function PlatformAdminPage() {
  const { data, isLoading } = useOrganizations();
  const createOrg = useCreateOrganization();
  const archiveOrg = useArchiveOrganizationById();
  const deleteOrg = useDeleteOrganization();
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [formError, setFormError] = useState<string | null>(null);
  const [confirming, setConfirming] = useState<Confirming | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setFormError(null);
    try {
      const org = await createOrg.mutateAsync({ name, slug });
      setName("");
      setSlug("");
      navigate(`/orgs/${org.id}`);
    } catch {
      setFormError("Failed to create organization.");
    }
  }

  async function onArchive(orgId: string) {
    setActionError(null);
    try {
      await archiveOrg.mutateAsync(orgId);
      setConfirming(null);
    } catch {
      // A 403 here just means the caller isn't that org's own Owner -
      // being a platform admin grants create/delete-gating at the
      // *organization:create* layer, not organization:delete (Owner
      // only) for every org that happens to exist - the client doesn't
      // pre-check locally, it surfaces the real backend rejection.
      setActionError("Failed to archive - this action requires the organization:delete permission (Owner only).");
    }
  }

  async function onDelete(orgId: string) {
    setActionError(null);
    try {
      await deleteOrg.mutateAsync(orgId);
      setConfirming(null);
    } catch {
      setActionError("Failed to delete - this action requires the organization:delete permission (Owner only).");
    }
  }

  function matchesConfirmationText(org: Organization, typed: string) {
    const normalized = typed.trim().toLowerCase();
    return normalized === org.name.toLowerCase() || normalized === org.slug.toLowerCase();
  }

  return (
    <div>
      <div className="page-header">
        <h1>Organizations</h1>
      </div>

      {actionError && <div className="error-banner">{actionError}</div>}
      {isLoading && <p className="muted">Loading…</p>}
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Slug</th>
            <th>Status</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {data?.data.map((org) => {
            const isConfirmingThis = confirming?.orgId === org.id;
            return (
              <tr key={org.id}>
                <td style={{ cursor: "pointer" }} onClick={() => navigate(`/orgs/${org.id}`)}>
                  {org.name}
                </td>
                <td className="mono">{org.slug}</td>
                <td>
                  <span className={`badge ${org.status === "archived" ? "badge-warning" : "badge-success"}`}>
                    {org.status}
                  </span>
                </td>
                <td>
                  {isConfirmingThis && confirming.action === "archive" ? (
                    <>
                      <button className="danger" onClick={() => onArchive(org.id)} disabled={archiveOrg.isPending}>
                        Confirm archive
                      </button>{" "}
                      <button className="secondary" onClick={() => setConfirming(null)}>
                        Cancel
                      </button>
                    </>
                  ) : isConfirmingThis && confirming.action === "delete" ? (
                    <div style={{ display: "flex", flexDirection: "column", gap: 6, alignItems: "flex-end" }}>
                      <span className="muted" style={{ fontSize: 12 }}>
                        Type "{org.slug}" to permanently delete
                      </span>
                      <input
                        autoFocus
                        value={confirming.typed}
                        onChange={(e) => setConfirming({ orgId: org.id, action: "delete", typed: e.target.value })}
                        style={{ width: 180 }}
                      />
                      <div>
                        <button
                          className="danger"
                          onClick={() => onDelete(org.id)}
                          disabled={deleteOrg.isPending || !matchesConfirmationText(org, confirming.typed)}
                        >
                          Confirm delete
                        </button>{" "}
                        <button className="secondary" onClick={() => setConfirming(null)}>
                          Cancel
                        </button>
                      </div>
                    </div>
                  ) : (
                    <>
                      {org.status !== "archived" && (
                        <button className="danger" onClick={() => setConfirming({ orgId: org.id, action: "archive" })}>
                          Archive
                        </button>
                      )}{" "}
                      <button
                        className="danger"
                        onClick={() => setConfirming({ orgId: org.id, action: "delete", typed: "" })}
                      >
                        Delete
                      </button>
                    </>
                  )}
                </td>
              </tr>
            );
          })}
          {data?.data.length === 0 && (
            <tr>
              <td colSpan={4} className="muted">
                None yet.
              </td>
            </tr>
          )}
        </tbody>
      </table>

      <div className="card" style={{ marginTop: 20, maxWidth: 480 }}>
        <h3>Create organization</h3>
        {formError && <div className="error-banner">{formError}</div>}
        <form onSubmit={onSubmit}>
          <label>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} required />
          </label>
          <label>
            Slug
            <input value={slug} onChange={(e) => setSlug(e.target.value)} required />
          </label>
          <button type="submit" disabled={createOrg.isPending}>
            Create
          </button>
        </form>
      </div>
    </div>
  );
}

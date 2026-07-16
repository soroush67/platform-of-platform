import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";

import { useCreateOrganization, useOrganizations } from "../api/hooks/useTenancy";

// PlatformAdminPage is where Organization creation actually lives now -
// inside the app, for platform admins only (internal/tenancy/application/
// create_organization.go's own gate), not as a screen every user has to
// pass through before reaching anything else.
export function PlatformAdminPage() {
  const { data, isLoading } = useOrganizations();
  const createOrg = useCreateOrganization();
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

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

  return (
    <div className="login-shell" style={{ flexDirection: "column", gap: 24 }}>
      <div className="card" style={{ width: 480 }}>
        <h1>Platform admin</h1>
        <p className="muted">Organizations you belong to:</p>
        {isLoading && <p className="muted">Loading…</p>}
        {data && data.data.length === 0 && <p className="muted">None yet - create the first one below.</p>}
        {data && data.data.length > 0 && (
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

      <div className="card" style={{ width: 480 }}>
        <h2>Create organization</h2>
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

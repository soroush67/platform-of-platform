import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";

import { useCreateOrganization, useOrganizations } from "../api/hooks/useTenancy";

export function OrgListPage() {
  const { data, isLoading, error } = useOrganizations();
  const createOrg = useCreateOrganization();
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    const org = await createOrg.mutateAsync({ name, slug });
    setName("");
    setSlug("");
    navigate(`/orgs/${org.id}`);
  }

  return (
    <div className="login-shell" style={{ flexDirection: "column", gap: 24 }}>
      <div className="card" style={{ width: 480 }}>
        <h1>Your organizations</h1>
        {isLoading && <p className="muted">Loading…</p>}
        {error && <div className="error-banner">Failed to load organizations.</div>}
        {data && data.data.length === 0 && <p className="muted">You aren't a member of any organization yet.</p>}
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

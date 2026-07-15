import { useState, type FormEvent } from "react";
import { Link, useParams } from "react-router-dom";

import { useCreateServiceAccount } from "../api/hooks/useIdentity";
import type { ServiceAccount } from "../api/types";

// ServiceAccountsPage accumulates known service accounts from create
// responses only - no list endpoint exists in this API today, same
// honest-scoping choice as TeamsPage/MembersPage.
export function ServiceAccountsPage() {
  const { orgId = "" } = useParams();
  const createSA = useCreateServiceAccount(orgId);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [known, setKnown] = useState<ServiceAccount[]>([]);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    const sa = await createSA.mutateAsync({ name, description });
    setKnown((prev) => [...prev, sa]);
    setName("");
    setDescription("");
  }

  return (
    <div>
      <div className="page-header">
        <h1>Service accounts</h1>
      </div>
      <p className="muted">
        No service-account-roster endpoint exists in this API yet - this page only shows accounts created this
        session.
      </p>

      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          {known.map((sa) => (
            <tr key={sa.id}>
              <td>
                <Link to={`/orgs/${orgId}/service-accounts/${sa.id}`}>{sa.name}</Link>
              </td>
              <td className="muted">{sa.description}</td>
            </tr>
          ))}
          {known.length === 0 && (
            <tr>
              <td colSpan={2} className="muted">
                None created this session yet.
              </td>
            </tr>
          )}
        </tbody>
      </table>

      <div className="card" style={{ marginTop: 20, maxWidth: 480 }}>
        <h3>Create service account</h3>
        <form onSubmit={onSubmit}>
          <label>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} required />
          </label>
          <label>
            Description
            <input value={description} onChange={(e) => setDescription(e.target.value)} />
          </label>
          <button type="submit" disabled={createSA.isPending}>
            Create
          </button>
        </form>
      </div>
    </div>
  );
}

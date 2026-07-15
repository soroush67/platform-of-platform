import { useState, type FormEvent } from "react";
import { Link, useParams } from "react-router-dom";

import { useCreateProject, useProjects } from "../api/hooks/useTenancy";

export function ProjectListPage() {
  const { orgId = "" } = useParams();
  const { data, isLoading } = useProjects(orgId);
  const createProject = useCreateProject(orgId);
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [description, setDescription] = useState("");

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    await createProject.mutateAsync({ name, slug, description });
    setName("");
    setSlug("");
    setDescription("");
  }

  return (
    <div>
      <div className="page-header">
        <h1>Projects</h1>
      </div>

      {isLoading && <p className="muted">Loading…</p>}
      {data && (
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Slug</th>
              <th>Description</th>
            </tr>
          </thead>
          <tbody>
            {data.data.map((p) => (
              <tr key={p.id}>
                <td>
                  <Link to={`/orgs/${orgId}/projects/${p.id}`}>{p.name}</Link>
                </td>
                <td className="mono">{p.slug}</td>
                <td className="muted">{p.description}</td>
              </tr>
            ))}
            {data.data.length === 0 && (
              <tr>
                <td colSpan={3} className="muted">
                  No projects yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      )}

      <div className="card" style={{ marginTop: 20, maxWidth: 480 }}>
        <h3>Create project</h3>
        {createProject.isError && <div className="error-banner">Failed to create project.</div>}
        <form onSubmit={onSubmit}>
          <label>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} required />
          </label>
          <label>
            Slug
            <input value={slug} onChange={(e) => setSlug(e.target.value)} required />
          </label>
          <label>
            Description
            <input value={description} onChange={(e) => setDescription(e.target.value)} />
          </label>
          <button type="submit" disabled={createProject.isPending}>
            Create
          </button>
        </form>
      </div>
    </div>
  );
}

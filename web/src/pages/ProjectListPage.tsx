import { useEffect, useState, type FormEvent } from "react";
import { Link, useParams } from "react-router-dom";

import { useCreateProject, useDeleteProject, useProjects } from "../api/hooks/useTenancy";

export function ProjectListPage() {
  const { orgId = "" } = useParams();
  const { data, isLoading } = useProjects(orgId);
  const createProject = useCreateProject(orgId);
  const deleteProject = useDeleteProject(orgId);
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [description, setDescription] = useState("");

  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  // confirmingDelete - same two-step confirm pattern as
  // ComposeFileDetailPage's destructive-operation trigger, avoiding a
  // native window.confirm. Resets whenever the selection changes so a
  // stale confirm can't fire against a different set of projects.
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  useEffect(() => {
    setConfirmingDelete(false);
  }, [selectedIds]);

  function toggleSelected(id: string) {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function toggleSelectAll() {
    if (!data) return;
    setSelectedIds((prev) => (prev.size === data.data.length ? new Set() : new Set(data.data.map((p) => p.id))));
  }

  async function onDeleteSelected() {
    if (!confirmingDelete) {
      setConfirmingDelete(true);
      return;
    }
    setDeleteError(null);
    const ids = Array.from(selectedIds);
    const results = await Promise.allSettled(ids.map((id) => deleteProject.mutateAsync(id)));
    const failed = results.filter((r) => r.status === "rejected").length;
    if (failed > 0) {
      setDeleteError(`Failed to delete ${failed} of ${ids.length} project(s).`);
    }
    setSelectedIds(new Set());
    setConfirmingDelete(false);
  }

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
        {selectedIds.size > 0 &&
          (confirmingDelete ? (
            <>
              <span className="muted">Permanently delete {selectedIds.size} project(s) and everything in them?</span>{" "}
              <button className="danger" onClick={onDeleteSelected} disabled={deleteProject.isPending}>
                {deleteProject.isPending ? "Deleting…" : "Confirm delete"}
              </button>{" "}
              <button className="secondary" onClick={() => setConfirmingDelete(false)}>
                Cancel
              </button>
            </>
          ) : (
            <button className="danger" onClick={onDeleteSelected}>
              Delete selected ({selectedIds.size})
            </button>
          ))}
      </div>

      {deleteError && <div className="error-banner">{deleteError}</div>}
      {isLoading && <p className="muted">Loading…</p>}
      {data && (
        <table>
          <thead>
            <tr>
              <th>
                <input
                  type="checkbox"
                  checked={data.data.length > 0 && selectedIds.size === data.data.length}
                  onChange={toggleSelectAll}
                />
              </th>
              <th>Name</th>
              <th>Slug</th>
              <th>Description</th>
            </tr>
          </thead>
          <tbody>
            {data.data.map((p) => (
              <tr key={p.id}>
                <td>
                  <input type="checkbox" checked={selectedIds.has(p.id)} onChange={() => toggleSelected(p.id)} />
                </td>
                <td>
                  <Link to={`/orgs/${orgId}/projects/${p.id}`}>{p.name}</Link>
                </td>
                <td className="mono">{p.slug}</td>
                <td className="muted">{p.description}</td>
              </tr>
            ))}
            {data.data.length === 0 && (
              <tr>
                <td colSpan={4} className="muted">
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

import { useState, type FormEvent } from "react";
import { Link, useParams } from "react-router-dom";

import { useComposeFiles, useCreateComposeFile, useDeleteComposeFile } from "../api/hooks/useFleet";

export function ComposeFilesPage() {
  const { orgId = "" } = useParams();
  const { data: composeFiles, isLoading } = useComposeFiles(orgId);
  const createComposeFile = useCreateComposeFile(orgId);
  const deleteComposeFile = useDeleteComposeFile(orgId);

  const [name, setName] = useState("");
  const [isGlobal, setIsGlobal] = useState(false);
  const [content, setContent] = useState("services:\n  app:\n    image: nginx:latest\n");
  const [formError, setFormError] = useState<string | null>(null);
  // confirmingDeleteId - two-step confirm, one row at a time (same
  // pattern as MachinesPage's Delete button).
  const [confirmingDeleteId, setConfirmingDeleteId] = useState<string | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  async function onDelete(composeFileId: string) {
    if (confirmingDeleteId !== composeFileId) {
      setConfirmingDeleteId(composeFileId);
      setDeleteError(null);
      return;
    }
    try {
      await deleteComposeFile.mutateAsync(composeFileId);
    } catch {
      setDeleteError("Failed to delete - this compose file has deploy (Operation) history and can't be deleted.");
    }
    setConfirmingDeleteId(null);
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setFormError(null);
    try {
      await createComposeFile.mutateAsync({ name, compose_content: content, is_global: isGlobal });
      setName("");
      setIsGlobal(false);
    } catch {
      setFormError("Failed to create compose file - check that it's valid YAML with at least one service (image or build).");
    }
  }

  return (
    <div>
      <div className="page-header">
        <h1>Compose files</h1>
      </div>

      {deleteError && <div className="error-banner">{deleteError}</div>}
      {isLoading && <p className="muted">Loading…</p>}
      {composeFiles && (
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Global</th>
              <th>Created</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {composeFiles.data.map((c) => (
              <tr key={c.id}>
                <td>
                  <Link to={`/orgs/${orgId}/compose-files/${c.id}`}>{c.name}</Link>
                </td>
                <td>{c.is_global && <span className="badge">global</span>}</td>
                <td className="muted">{new Date(c.created_at).toLocaleString()}</td>
                <td>
                  {confirmingDeleteId === c.id ? (
                    <>
                      <button className="danger" onClick={() => onDelete(c.id)} disabled={deleteComposeFile.isPending}>
                        {deleteComposeFile.isPending ? "Deleting…" : "Confirm delete"}
                      </button>{" "}
                      <button className="secondary" onClick={() => setConfirmingDeleteId(null)}>
                        Cancel
                      </button>
                    </>
                  ) : (
                    <button className="danger" onClick={() => onDelete(c.id)}>
                      Delete
                    </button>
                  )}
                </td>
              </tr>
            ))}
            {composeFiles.data.length === 0 && (
              <tr>
                <td colSpan={4} className="muted">
                  No compose files yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      )}

      <div className="card" style={{ marginTop: 20, maxWidth: 640 }}>
        <h3>Create compose file</h3>
        <p className="muted">
          A global compose file's variables are used as a fallback for every other compose file's own unresolved keys.
        </p>
        {formError && <div className="error-banner">{formError}</div>}
        <form onSubmit={onSubmit}>
          <label>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} required />
          </label>
          <label style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
            <input type="checkbox" checked={isGlobal} onChange={(e) => setIsGlobal(e.target.checked)} />
            Global (org-wide variable fallback - only one allowed per org)
          </label>
          <label>
            docker-compose.yml
            <textarea
              className="mono"
              value={content}
              onChange={(e) => setContent(e.target.value)}
              rows={10}
              required
            />
          </label>
          <button type="submit" disabled={createComposeFile.isPending}>
            Create
          </button>
        </form>
      </div>
    </div>
  );
}

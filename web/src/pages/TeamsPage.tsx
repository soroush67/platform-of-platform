import { useState, type FormEvent } from "react";
import { useParams } from "react-router-dom";

import {
  useAddTeamMember,
  useCreateTeam,
  useDeleteTeam,
  useMembers,
  useRemoveTeamMember,
  useTeamMembers,
  useTeams,
  useUpdateTeam,
} from "../api/hooks/useTenancy";
import type { Team } from "../api/types";

function TeamCard({ orgId, team }: { orgId: string; team: Team }) {
  const { data: members } = useMembers(orgId);
  const { data: teamMembers, isLoading: rosterLoading } = useTeamMembers(orgId, team.id);
  const addMember = useAddTeamMember(orgId, team.id);
  const removeMember = useRemoveTeamMember(orgId, team.id);
  const updateTeam = useUpdateTeam(orgId);
  const deleteTeam = useDeleteTeam(orgId);
  const [userId, setUserId] = useState("");
  const [error, setError] = useState<string | null>(null);

  const [renaming, setRenaming] = useState(false);
  const [newName, setNewName] = useState(team.name);
  const [confirmingDelete, setConfirmingDelete] = useState(false);

  const rosterUserIds = new Set(teamMembers?.data.map((m) => m.user_id));
  const addableMembers = members?.data.filter((m) => !rosterUserIds.has(m.user_id));

  async function onAdd() {
    setError(null);
    try {
      await addMember.mutateAsync(userId);
      setUserId("");
    } catch {
      setError("Failed to add member.");
    }
  }

  async function onRemove(removeUserId: string) {
    setError(null);
    try {
      await removeMember.mutateAsync(removeUserId);
    } catch {
      setError("Failed to remove member.");
    }
  }

  async function onRename() {
    setError(null);
    try {
      await updateTeam.mutateAsync({ teamId: team.id, name: newName });
      setRenaming(false);
    } catch {
      setError("Failed to rename team - the name may already be taken.");
    }
  }

  async function onDelete() {
    setError(null);
    try {
      await deleteTeam.mutateAsync(team.id);
    } catch {
      setError("Failed to delete team.");
    }
  }

  return (
    <div className="card">
      <div className="field-row" style={{ justifyContent: "space-between", alignItems: "center" }}>
        {renaming ? (
          <div className="field-row">
            <input value={newName} onChange={(e) => setNewName(e.target.value)} />
            <button className="secondary" disabled={!newName || updateTeam.isPending} onClick={onRename}>
              Save
            </button>
            <button
              className="secondary"
              onClick={() => {
                setRenaming(false);
                setNewName(team.name);
              }}
            >
              Cancel
            </button>
          </div>
        ) : (
          <h3 style={{ margin: 0 }}>
            {team.name}{" "}
            <button className="secondary" onClick={() => setRenaming(true)}>
              Rename
            </button>
          </h3>
        )}

        {confirmingDelete ? (
          <div>
            <button className="danger" disabled={deleteTeam.isPending} onClick={onDelete}>
              Confirm delete team
            </button>{" "}
            <button className="secondary" onClick={() => setConfirmingDelete(false)}>
              Cancel
            </button>
          </div>
        ) : (
          <button className="danger" onClick={() => setConfirmingDelete(true)}>
            Delete team
          </button>
        )}
      </div>
      <p className="muted mono">{team.id}</p>

      {error && <div className="error-banner">{error}</div>}

      {rosterLoading && <p className="muted">Loading members…</p>}
      {teamMembers && (
        <table>
          <thead>
            <tr>
              <th>Username</th>
              <th>Email</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {teamMembers.data.map((m) => (
              <tr key={m.user_id}>
                <td>{m.username || <span className="mono">{m.user_id}</span>}</td>
                <td>{m.email}</td>
                <td>
                  <button className="danger" disabled={removeMember.isPending} onClick={() => onRemove(m.user_id)}>
                    Remove
                  </button>
                </td>
              </tr>
            ))}
            {teamMembers.data.length === 0 && (
              <tr>
                <td colSpan={3} className="muted">
                  No members yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      )}

      <div className="field-row" style={{ marginTop: 8 }}>
        <select value={userId} onChange={(e) => setUserId(e.target.value)}>
          <option value="">— choose member —</option>
          {addableMembers?.map((m) => (
            <option key={m.user_id} value={m.user_id}>
              {m.username}
            </option>
          ))}
        </select>
        <button className="secondary" disabled={!userId || addMember.isPending} onClick={onAdd}>
          Add member
        </button>
      </div>
    </div>
  );
}

// Teams are the "Group" concept RoleBindingsPage's User/Group access
// control binds Roles to (see the RBAC per-menu access-control
// redesign) - a real roster via useTeams, not just what was created
// this session.
export function TeamsPage() {
  const { orgId = "" } = useParams();
  const { data: teams, isLoading } = useTeams(orgId);
  const createTeam = useCreateTeam(orgId);
  const [name, setName] = useState("");
  const [createError, setCreateError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setCreateError(null);
    try {
      await createTeam.mutateAsync(name);
      setName("");
    } catch {
      setCreateError("Failed to create team - the name may already be taken.");
    }
  }

  return (
    <div>
      <div className="page-header">
        <h1>Teams</h1>
      </div>

      {isLoading && <p className="muted">Loading…</p>}
      {teams?.data.length === 0 && <p className="muted">No teams yet.</p>}
      {teams?.data.map((t: Team) => (
        <TeamCard key={t.id} orgId={orgId} team={t} />
      ))}

      <div className="card" style={{ maxWidth: 480 }}>
        <h3>Create team</h3>
        {createError && <div className="error-banner">{createError}</div>}
        <form onSubmit={onSubmit}>
          <label>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} required />
          </label>
          <button type="submit" disabled={createTeam.isPending}>
            Create
          </button>
        </form>
      </div>
    </div>
  );
}

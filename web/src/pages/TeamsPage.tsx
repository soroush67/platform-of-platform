import { useState, type FormEvent } from "react";
import { useParams } from "react-router-dom";

import { useAddTeamMember, useCreateTeam, useRemoveTeamMember, useTeams } from "../api/hooks/useTenancy";
import type { Team } from "../api/types";

function TeamCard({ orgId, team }: { orgId: string; team: Team }) {
  const addMember = useAddTeamMember(orgId, team.id);
  const removeMember = useRemoveTeamMember(orgId, team.id);
  const [userId, setUserId] = useState("");

  return (
    <div className="card">
      <h3>{team.name}</h3>
      <p className="muted mono">{team.id}</p>
      <div className="field-row">
        <input placeholder="user id" value={userId} onChange={(e) => setUserId(e.target.value)} />
        <button
          className="secondary"
          disabled={!userId || addMember.isPending}
          onClick={() => addMember.mutate(userId)}
        >
          Add member
        </button>
        <button
          className="danger"
          disabled={!userId || removeMember.isPending}
          onClick={() => removeMember.mutate(userId)}
        >
          Remove member
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

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    await createTeam.mutateAsync(name);
    setName("");
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

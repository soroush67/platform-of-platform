import { useState, type FormEvent } from "react";
import { useParams } from "react-router-dom";

import { useAddTeamMember, useCreateTeam, useRemoveTeamMember } from "../api/hooks/useTenancy";
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

// TeamsPage accumulates known teams from create responses only - no
// `GET /orgs/{id}/teams` list endpoint exists in this API today,
// deliberately not invented for this UI (see the plan's own note on
// staying within the two backend endpoints actually added).
export function TeamsPage() {
  const { orgId = "" } = useParams();
  const createTeam = useCreateTeam(orgId);
  const [name, setName] = useState("");
  const [knownTeams, setKnownTeams] = useState<Team[]>([]);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    const team = await createTeam.mutateAsync(name);
    setKnownTeams((prev) => [...prev, team]);
    setName("");
  }

  return (
    <div>
      <div className="page-header">
        <h1>Teams</h1>
      </div>
      <p className="muted">
        No team-roster endpoint exists in this API yet - this page only shows teams created this session.
      </p>

      {knownTeams.map((t) => (
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

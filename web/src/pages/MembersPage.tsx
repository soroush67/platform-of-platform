import { useState } from "react";
import { useParams } from "react-router-dom";

import { useAddMember, useChangeMemberRole } from "../api/hooks/useTenancy";

// MembersPage is deliberately scoped down to "operate by id" forms, not
// a browsable roster - there is no `GET /orgs/{id}/members` endpoint in
// this API today (only add-member and change-role exist), and this UI
// build didn't invent a third backend endpoint beyond the two already
// added (GET /orgs, GET /users/me).
export function MembersPage() {
  const { orgId = "" } = useParams();
  const addMember = useAddMember(orgId);
  const changeRole = useChangeMemberRole(orgId);

  const [addUserId, setAddUserId] = useState("");
  const [addResult, setAddResult] = useState<string | null>(null);

  const [roleUserId, setRoleUserId] = useState("");
  const [role, setRole] = useState("read");
  const [roleResult, setRoleResult] = useState<string | null>(null);

  return (
    <div>
      <div className="page-header">
        <h1>Members</h1>
      </div>
      <p className="muted">No member-roster endpoint exists in this API yet - manage members by id below.</p>

      <div className="card" style={{ maxWidth: 480 }}>
        <h3>Add member</h3>
        {addResult && <div className="error-banner">{addResult}</div>}
        <form
          onSubmit={async (e) => {
            e.preventDefault();
            setAddResult(null);
            try {
              await addMember.mutateAsync(addUserId);
              setAddResult("Added.");
              setAddUserId("");
            } catch {
              setAddResult("Failed to add member.");
            }
          }}
        >
          <label>
            User ID
            <input value={addUserId} onChange={(e) => setAddUserId(e.target.value)} required />
          </label>
          <button type="submit" disabled={addMember.isPending}>
            Add
          </button>
        </form>
      </div>

      <div className="card" style={{ maxWidth: 480 }}>
        <h3>Change member role</h3>
        {roleResult && <div className="error-banner">{roleResult}</div>}
        <form
          onSubmit={async (e) => {
            e.preventDefault();
            setRoleResult(null);
            try {
              await changeRole.mutateAsync({ userId: roleUserId, role });
              setRoleResult("Updated.");
            } catch {
              setRoleResult("Failed to change role.");
            }
          }}
        >
          <label>
            User ID
            <input value={roleUserId} onChange={(e) => setRoleUserId(e.target.value)} required />
          </label>
          <label>
            Role name
            <input value={role} onChange={(e) => setRole(e.target.value)} required />
          </label>
          <button type="submit" disabled={changeRole.isPending}>
            Update
          </button>
        </form>
      </div>
    </div>
  );
}

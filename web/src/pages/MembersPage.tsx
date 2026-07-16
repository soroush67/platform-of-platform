import { useState, type FormEvent } from "react";
import { useParams } from "react-router-dom";

import { useAddMember, useChangeMemberRole, useMembers } from "../api/hooks/useTenancy";
import { useCreateUser } from "../api/hooks/useIdentity";
import { ROLE_NAMES, type RoleName } from "../api/types";

// MemberRoleSelect is the roster table's own inline "change role" action -
// acts on a known user_id (unlike the old free-text-by-ID form this
// page used to have), and is a real dropdown over the 4 role names
// ChangeMemberRoleService actually accepts, not free text.
function MemberRoleSelect({ orgId, userId, currentRole }: { orgId: string; userId: string; currentRole: string }) {
  const changeRole = useChangeMemberRole(orgId);
  const [error, setError] = useState<string | null>(null);

  return (
    <div>
      <select
        value={ROLE_NAMES.includes(currentRole as RoleName) ? currentRole : ""}
        disabled={changeRole.isPending}
        onChange={async (e) => {
          setError(null);
          try {
            await changeRole.mutateAsync({ userId, role: e.target.value });
          } catch {
            setError("Failed to change role.");
          }
        }}
      >
        {!ROLE_NAMES.includes(currentRole as RoleName) && (
          <option value="" disabled>
            {currentRole || "no role bound"}
          </option>
        )}
        {ROLE_NAMES.map((r) => (
          <option key={r} value={r}>
            {r}
          </option>
        ))}
      </select>
      {error && <div className="error-banner">{error}</div>}
    </div>
  );
}

export function MembersPage() {
  const { orgId = "" } = useParams();
  const { data: members, isLoading } = useMembers(orgId);
  const createUser = useCreateUser();
  const addMember = useAddMember(orgId);
  const changeRole = useChangeMemberRole(orgId);

  // ---- Create user & add to this org ----
  const [newUsername, setNewUsername] = useState("");
  const [newEmail, setNewEmail] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [newRole, setNewRole] = useState<RoleName>("read");
  const [createError, setCreateError] = useState<string | null>(null);
  const [createSuccess, setCreateSuccess] = useState<string | null>(null);
  // orphanedUserId: the user was created but the follow-up add-to-org
  // (or role change) call failed - kept so "Retry" can finish the flow
  // without minting a second, duplicate user.
  const [orphanedUserId, setOrphanedUserId] = useState<string | null>(null);

  async function finishAddingUser(userId: string, role: RoleName) {
    await addMember.mutateAsync(userId);
    if (role !== "read") {
      await changeRole.mutateAsync({ userId, role });
    }
  }

  async function onCreateUser(e: FormEvent) {
    e.preventDefault();
    setCreateError(null);
    setCreateSuccess(null);
    try {
      const user = await createUser.mutateAsync({ username: newUsername, email: newEmail, password: newPassword });
      try {
        await finishAddingUser(user.id, newRole);
        setCreateSuccess(`Created ${user.username} and added to this org as ${newRole}.`);
        setOrphanedUserId(null);
        setNewUsername("");
        setNewEmail("");
        setNewPassword("");
        setNewRole("read");
      } catch {
        // The user row is real and already exists - don't drop its id,
        // let Retry below finish just the add-to-org step.
        setOrphanedUserId(user.id);
        setCreateError(`User ${user.username} was created but could not be added to this org.`);
      }
    } catch {
      setCreateError("Failed to create user.");
    }
  }

  async function onRetryAdd() {
    if (!orphanedUserId) return;
    setCreateError(null);
    try {
      await finishAddingUser(orphanedUserId, newRole);
      setCreateSuccess("Added to this org.");
      setOrphanedUserId(null);
    } catch {
      setCreateError("Still failed to add to this org - the user id is preserved, you can retry again.");
    }
  }

  // ---- Add existing user by ID ----
  const [existingUserId, setExistingUserId] = useState("");
  const [existingResult, setExistingResult] = useState<string | null>(null);

  return (
    <div>
      <div className="page-header">
        <h1>Members</h1>
      </div>

      {isLoading && <p className="muted">Loading…</p>}
      {members && (
        <table>
          <thead>
            <tr>
              <th>Username</th>
              <th>Email</th>
              <th>Role</th>
              <th>Joined</th>
            </tr>
          </thead>
          <tbody>
            {members.data.map((m) => (
              <tr key={m.user_id}>
                <td>{m.username || <span className="mono">{m.user_id}</span>}</td>
                <td>{m.email}</td>
                <td>
                  <MemberRoleSelect orgId={orgId} userId={m.user_id} currentRole={m.role_name} />
                </td>
                <td className="muted">{new Date(m.joined_at).toLocaleDateString()}</td>
              </tr>
            ))}
            {members.data.length === 0 && (
              <tr>
                <td colSpan={4} className="muted">
                  No members yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      )}

      <div className="card" style={{ marginTop: 20, maxWidth: 480 }}>
        <h3>Create user &amp; add to this org</h3>
        {createError && <div className="error-banner">{createError}</div>}
        {createSuccess && <p className="muted">{createSuccess}</p>}
        {orphanedUserId ? (
          <button type="button" onClick={onRetryAdd} disabled={addMember.isPending || changeRole.isPending}>
            Retry adding to this org
          </button>
        ) : (
          <form onSubmit={onCreateUser}>
            <label>
              Username
              <input value={newUsername} onChange={(e) => setNewUsername(e.target.value)} required />
            </label>
            <label>
              Email
              <input type="email" value={newEmail} onChange={(e) => setNewEmail(e.target.value)} required />
            </label>
            <label>
              Password
              <input type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} required />
            </label>
            <label>
              Role
              <select value={newRole} onChange={(e) => setNewRole(e.target.value as RoleName)}>
                {ROLE_NAMES.map((r) => (
                  <option key={r} value={r}>
                    {r}
                  </option>
                ))}
              </select>
            </label>
            <button type="submit" disabled={createUser.isPending || addMember.isPending || changeRole.isPending}>
              Create &amp; add
            </button>
          </form>
        )}
      </div>

      <div className="card" style={{ marginTop: 20, maxWidth: 480 }}>
        <h3>Add existing user by ID</h3>
        <p className="muted">
          For a user already created elsewhere (another org, or directly via the API) - there&apos;s no cross-org
          user search in this system yet, so this is the only way to attach a known user id to this org.
        </p>
        {existingResult && <div className="error-banner">{existingResult}</div>}
        <form
          onSubmit={async (e) => {
            e.preventDefault();
            setExistingResult(null);
            try {
              await addMember.mutateAsync(existingUserId);
              setExistingResult("Added.");
              setExistingUserId("");
            } catch {
              setExistingResult("Failed to add member.");
            }
          }}
        >
          <label>
            User ID
            <input value={existingUserId} onChange={(e) => setExistingUserId(e.target.value)} required />
          </label>
          <button type="submit" disabled={addMember.isPending}>
            Add
          </button>
        </form>
      </div>
    </div>
  );
}

import { useState, type FormEvent } from "react";
import { useParams } from "react-router-dom";

import {
  useAddMember,
  useAvailableUsers,
  useBlockMember,
  useChangeMemberRole,
  useMembers,
  useOrganization,
  useRemoveMember,
  useUnblockMember,
} from "../api/hooks/useTenancy";
import { useCreateUser } from "../api/hooks/useIdentity";
import { ApiError, ROLE_NAMES, type Member, type RoleName } from "../api/types";

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

// MemberRowActions - Block/Unblock (per-organization suspension only,
// see Member's own "blocked" field comment) and Remove (a real,
// permanent removal of this membership - the User account itself is
// untouched), in the same row the member is listed in.
function MemberRowActions({ orgId, member }: { orgId: string; member: Member }) {
  const blockMember = useBlockMember(orgId);
  const unblockMember = useUnblockMember(orgId);
  const removeMember = useRemoveMember(orgId);
  const [confirmingRemove, setConfirmingRemove] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function onToggleBlock() {
    setError(null);
    try {
      if (member.blocked) {
        await unblockMember.mutateAsync(member.user_id);
      } else {
        await blockMember.mutateAsync(member.user_id);
      }
    } catch {
      setError(member.blocked ? "Failed to unblock member." : "Failed to block member.");
    }
  }

  async function onRemove() {
    setError(null);
    try {
      await removeMember.mutateAsync(member.user_id);
    } catch {
      setError("Failed to remove member.");
    }
  }

  return (
    <div>
      <div className="field-row">
        <button
          className="secondary"
          disabled={blockMember.isPending || unblockMember.isPending}
          onClick={onToggleBlock}
        >
          {member.blocked ? "Unblock" : "Block"}
        </button>
        {confirmingRemove ? (
          <>
            <button className="danger" disabled={removeMember.isPending} onClick={onRemove}>
              Confirm remove
            </button>
            <button className="secondary" onClick={() => setConfirmingRemove(false)}>
              Cancel
            </button>
          </>
        ) : (
          <button className="danger" onClick={() => setConfirmingRemove(true)}>
            Remove
          </button>
        )}
      </div>
      {error && <div className="error-banner">{error}</div>}
    </div>
  );
}

// AddExistingUserCard - the picker for a platform User that already
// exists (created here or in another org, or left orphaned by a prior
// "create user & add" whose add-to-org step failed) but isn't a member
// of this org yet. Without this, the only way in was "create user"
// (which 409s on an existing username with no recovery) or manually
// knowing the target's raw user_id.
function AddExistingUserCard({ orgId, orgName }: { orgId: string; orgName: string }) {
  const { data: available, isLoading } = useAvailableUsers(orgId);
  const addMember = useAddMember(orgId);
  const changeRole = useChangeMemberRole(orgId);
  const [selectedUserId, setSelectedUserId] = useState("");
  const [role, setRole] = useState<RoleName>("read");
  const [error, setError] = useState<string | null>(null);

  async function onAdd(e: FormEvent) {
    e.preventDefault();
    if (!selectedUserId) return;
    setError(null);
    try {
      await addMember.mutateAsync(selectedUserId);
      if (role !== "read") {
        await changeRole.mutateAsync({ userId: selectedUserId, role });
      }
      setSelectedUserId("");
      setRole("read");
    } catch {
      setError("Failed to add this user to the org.");
    }
  }

  if (!isLoading && available && available.data.length === 0) {
    return null;
  }

  return (
    <div className="card" style={{ marginTop: 20, maxWidth: 480 }}>
      <h3>Add existing user to {orgName}</h3>
      <p className="muted">
        Platform users who aren&apos;t members of this org yet - including any user already created (here or
        elsewhere) but never added to this org.
      </p>
      {isLoading && <p className="muted">Loading…</p>}
      {available && (
        <form onSubmit={onAdd}>
          <label>
            User
            <select value={selectedUserId} onChange={(e) => setSelectedUserId(e.target.value)} required>
              <option value="" disabled>
                Select a user…
              </option>
              {available.data.map((u) => (
                <option key={u.id} value={u.id}>
                  {u.username} ({u.email})
                </option>
              ))}
            </select>
          </label>
          <label>
            Role
            <select value={role} onChange={(e) => setRole(e.target.value as RoleName)}>
              {ROLE_NAMES.map((r) => (
                <option key={r} value={r}>
                  {r}
                </option>
              ))}
            </select>
          </label>
          <button type="submit" disabled={!selectedUserId || addMember.isPending || changeRole.isPending}>
            Add to {orgName}
          </button>
        </form>
      )}
      {error && <div className="error-banner">{error}</div>}
    </div>
  );
}

export function MembersPage() {
  const { orgId = "" } = useParams();
  const { data: org } = useOrganization(orgId);
  const { data: members, isLoading } = useMembers(orgId);
  const createUser = useCreateUser();
  const addMember = useAddMember(orgId);
  const changeRole = useChangeMemberRole(orgId);
  const orgName = org?.name || "this org";

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
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setCreateError(`A user named "${newUsername}" already exists - use "Add existing user" above instead.`);
      } else {
        setCreateError(err instanceof ApiError ? err.detail || err.message : "Failed to create user.");
      }
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

  return (
    <div>
      <div className="page-header">
        <h1>Members of {orgName}</h1>
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
              <th></th>
            </tr>
          </thead>
          <tbody>
            {members.data.map((m) => (
              <tr key={m.user_id}>
                <td>
                  {m.username || <span className="mono">{m.user_id}</span>}{" "}
                  {m.blocked && <span className="badge badge-warning">blocked</span>}
                </td>
                <td>{m.email}</td>
                <td>
                  <MemberRoleSelect orgId={orgId} userId={m.user_id} currentRole={m.role_name} />
                </td>
                <td className="muted">{new Date(m.joined_at).toLocaleDateString()}</td>
                <td>
                  <MemberRowActions orgId={orgId} member={m} />
                </td>
              </tr>
            ))}
            {members.data.length === 0 && (
              <tr>
                <td colSpan={5} className="muted">
                  No members yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      )}

      <AddExistingUserCard orgId={orgId} orgName={orgName} />

      <div className="card" style={{ marginTop: 20, maxWidth: 480 }}>
        <h3>Create new user &amp; add to {orgName}</h3>
        <p className="muted">
          For a user who doesn&apos;t exist on the platform yet. If they already have an account, use &quot;Add
          existing user&quot; above instead - usernames are unique platform-wide, so creating one again will fail.
        </p>
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
    </div>
  );
}

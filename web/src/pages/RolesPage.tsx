import { useState, type FormEvent } from "react";
import { useParams } from "react-router-dom";

import { useCreateRole, useRoles, useUpdateRole } from "../api/hooks/useRbac";
import { PERMISSIONS, PERMISSION_GROUPS } from "../api/types";
import type { Role } from "../api/types";

// GroupedPermissionPicker is shared by the create and edit forms below -
// one collapsible section per resource (Machines/Projects/Workspaces/...),
// each with its own "select all in group" checkbox, plus one global
// "select all" checkbox - replaces the old flat, unlabeled 16-checkbox
// wall.
function GroupedPermissionPicker({
  permissions,
  onToggle,
  onSetGroup,
  onSetAll,
}: {
  permissions: Set<string>;
  onToggle: (p: string) => void;
  onSetGroup: (groupPermissions: string[], checked: boolean) => void;
  onSetAll: (checked: boolean) => void;
}) {
  const allChecked = permissions.size === PERMISSIONS.length;

  return (
    <div>
      <label style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
        <input type="checkbox" checked={allChecked} onChange={(e) => onSetAll(e.target.checked)} />
        <strong>Select all</strong>
      </label>
      {PERMISSION_GROUPS.map((group) => {
        const groupChecked = group.permissions.every((p) => permissions.has(p));
        return (
          <div key={group.key} className="card" style={{ marginTop: 8, padding: 12 }}>
            <label style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
              <input
                type="checkbox"
                checked={groupChecked}
                onChange={(e) => onSetGroup(group.permissions, e.target.checked)}
              />
              <strong>{group.label}</strong>
            </label>
            <div style={{ marginLeft: 24 }}>
              {group.permissions.map((p) => (
                <label key={p} style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
                  <input type="checkbox" checked={permissions.has(p)} onChange={() => onToggle(p)} />
                  <span className="mono">{p.split(":")[1]}</span>
                </label>
              ))}
            </div>
          </div>
        );
      })}
    </div>
  );
}

// RoleRow's own grouped, friendlier permission display - "Machines:
// read, manage" chips instead of a flat wall of raw "machine:read"/
// "machine:manage" badge strings.
function GroupedPermissionBadges({ permissions }: { permissions: string[] }) {
  const set = new Set(permissions);
  const groupsWithAny = PERMISSION_GROUPS.filter((g) => g.permissions.some((p) => set.has(p)));

  return (
    <>
      {groupsWithAny.map((group) => {
        const actions = group.permissions.filter((p) => set.has(p)).map((p) => p.split(":")[1]);
        return (
          <span key={group.key} className="badge" style={{ marginRight: 4 }}>
            {group.label}: {actions.join(", ")}
          </span>
        );
      })}
    </>
  );
}

export function RolesPage() {
  const { orgId = "" } = useParams();
  const { data, isLoading } = useRoles(orgId);
  const createRole = useCreateRole(orgId);
  const updateRole = useUpdateRole(orgId);

  const [name, setName] = useState("");
  const [permissions, setPermissions] = useState<Set<string>>(new Set());
  const [editingRole, setEditingRole] = useState<Role | null>(null);
  const [formError, setFormError] = useState<string | null>(null);

  function togglePermission(p: string) {
    setPermissions((prev) => {
      const next = new Set(prev);
      if (next.has(p)) next.delete(p);
      else next.add(p);
      return next;
    });
  }

  function setGroup(groupPermissions: string[], checked: boolean) {
    setPermissions((prev) => {
      const next = new Set(prev);
      for (const p of groupPermissions) {
        if (checked) next.add(p);
        else next.delete(p);
      }
      return next;
    });
  }

  function setAll(checked: boolean) {
    setPermissions(checked ? new Set(PERMISSIONS) : new Set());
  }

  function startEdit(role: Role) {
    setEditingRole(role);
    setPermissions(new Set(role.permissions));
    setFormError(null);
  }

  function cancelEdit() {
    setEditingRole(null);
    setPermissions(new Set());
    setFormError(null);
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setFormError(null);
    try {
      if (editingRole) {
        await updateRole.mutateAsync({ roleId: editingRole.id, permissions: [...permissions] });
        setEditingRole(null);
      } else {
        await createRole.mutateAsync({ name, permissions: [...permissions] });
        setName("");
      }
      setPermissions(new Set());
    } catch {
      setFormError(editingRole ? "Failed to update role." : "Failed to create role - the name may already be taken.");
    }
  }

  return (
    <div>
      <div className="page-header">
        <h1>Roles</h1>
      </div>

      {isLoading && <p className="muted">Loading…</p>}
      {data && (
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Built-in</th>
              <th>Permissions</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {data.data.map((r) => (
              <tr key={r.id}>
                <td>{r.name}</td>
                <td>{r.organization_id === null ? <span className="badge badge-dim">built-in</span> : "—"}</td>
                <td>
                  <GroupedPermissionBadges permissions={r.permissions} />
                </td>
                <td>
                  {r.organization_id !== null && (
                    <button className="secondary" onClick={() => startEdit(r)}>
                      Edit
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <div className="card" style={{ marginTop: 20, maxWidth: 560 }}>
        <h3>{editingRole ? `Edit role: ${editingRole.name}` : "Create custom role"}</h3>
        {formError && <div className="error-banner">{formError}</div>}
        <form onSubmit={onSubmit}>
          {!editingRole && (
            <label>
              Name
              <input value={name} onChange={(e) => setName(e.target.value)} required />
            </label>
          )}
          <GroupedPermissionPicker
            permissions={permissions}
            onToggle={togglePermission}
            onSetGroup={setGroup}
            onSetAll={setAll}
          />
          <div className="field-row" style={{ marginTop: 12 }}>
            <button type="submit" disabled={createRole.isPending || updateRole.isPending || permissions.size === 0}>
              {editingRole ? "Save changes" : "Create"}
            </button>
            {editingRole && (
              <button type="button" className="secondary" onClick={cancelEdit}>
                Cancel
              </button>
            )}
          </div>
        </form>
      </div>
    </div>
  );
}

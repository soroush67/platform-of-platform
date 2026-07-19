import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { apiFetch } from "../client";
import type { ListResponse, Role, RoleBinding } from "../types";

export function useRoles(orgId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "roles"],
    queryFn: () => apiFetch<ListResponse<Role>>(`/orgs/${orgId}/roles`),
    enabled: !!orgId,
  });
}

export function useCreateRole(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; permissions: string[] }) =>
      apiFetch<Role>(`/orgs/${orgId}/roles`, { method: "POST", body: JSON.stringify(input) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "roles"] }),
  });
}

// useUpdateRole rewrites a custom Role's permission set in place
// (UpdateRoleService's own doc comment - name stays fixed, every
// existing binding to this Role picks up the new permissions
// immediately).
export function useUpdateRole(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ roleId, permissions }: { roleId: string; permissions: string[] }) =>
      apiFetch<Role>(`/orgs/${orgId}/roles/${roleId}`, { method: "PUT", body: JSON.stringify({ permissions }) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "roles"] }),
  });
}

export function useRoleBindings(orgId: string, subjectId?: string) {
  const qs = subjectId ? `?subject_id=${encodeURIComponent(subjectId)}` : "";
  return useQuery({
    queryKey: ["orgs", orgId, "role-bindings", subjectId ?? null],
    queryFn: () => apiFetch<ListResponse<RoleBinding>>(`/orgs/${orgId}/role-bindings${qs}`),
    enabled: !!orgId,
  });
}

export function useCreateRoleBinding(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: {
      role_id: string;
      subject: { type: string; id: string };
      scope: { type: string; id: string };
      effect?: string;
    }) => apiFetch<RoleBinding>(`/orgs/${orgId}/role-bindings`, { method: "POST", body: JSON.stringify(input) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "role-bindings"] }),
  });
}

// useDeleteRoleBinding is a real, permanent removal (operator-confirmed,
// not a client-side hide).
export function useDeleteRoleBinding(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (bindingId: string) =>
      apiFetch<void>(`/orgs/${orgId}/role-bindings/${bindingId}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "role-bindings"] }),
  });
}

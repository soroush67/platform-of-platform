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

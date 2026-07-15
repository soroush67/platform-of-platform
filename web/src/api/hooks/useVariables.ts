import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { apiFetch } from "../client";
import type { CreateVariableRequest, ListResponse, Variable } from "../types";

export function useVariables(orgId: string, scopeType: string, scopeId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "variables", scopeType, scopeId],
    queryFn: () =>
      apiFetch<ListResponse<Variable>>(
        `/orgs/${orgId}/variables?scope_type=${encodeURIComponent(scopeType)}&scope_id=${encodeURIComponent(scopeId)}`,
      ),
    enabled: !!orgId && !!scopeType && !!scopeId,
  });
}

export function useCreateVariable(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateVariableRequest) =>
      apiFetch<Variable>(`/orgs/${orgId}/variables`, { method: "POST", body: JSON.stringify(input) }),
    onSuccess: (_data, vars) =>
      qc.invalidateQueries({ queryKey: ["orgs", orgId, "variables", vars.scope_type, vars.scope_id] }),
  });
}

export function useUpdateVariable(orgId: string) {
  return useMutation({
    mutationFn: ({
      variableId,
      ...input
    }: {
      variableId: string;
      category: string;
      sensitivity: string;
      value: string;
    }) =>
      apiFetch<Variable>(`/orgs/${orgId}/variables/${variableId}`, {
        method: "PUT",
        body: JSON.stringify(input),
      }),
  });
}

export function useDeleteVariable(orgId: string) {
  return useMutation({
    mutationFn: (variableId: string) => apiFetch<void>(`/orgs/${orgId}/variables/${variableId}`, { method: "DELETE" }),
  });
}

// useResolveVariable is an on-demand mutation, not a query - resolving a
// secret_ref-backed variable is a real live Vault round-trip, so it's
// only ever triggered by an explicit user click ("Resolve" button), and
// its result is held in local component state, never written into the
// TanStack Query cache (the whole point of masking is that the real
// value shouldn't sit around any longer than the user asked for).
export function useResolveVariable(orgId: string, projectId: string, workspaceId: string) {
  return useMutation({
    mutationFn: (key: string) =>
      apiFetch<Variable>(
        `/orgs/${orgId}/projects/${projectId}/workspaces/${workspaceId}/variables/resolve?key=${encodeURIComponent(key)}`,
      ),
  });
}

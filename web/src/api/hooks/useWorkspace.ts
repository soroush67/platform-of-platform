import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { apiFetch } from "../client";
import type { Environment, ListResponse, Workspace } from "../types";

export function useEnvironments(orgId: string, projectId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "projects", projectId, "environments"],
    queryFn: () => apiFetch<ListResponse<Environment>>(`/orgs/${orgId}/projects/${projectId}/environments`),
    enabled: !!orgId && !!projectId,
  });
}

export function useEnvironment(orgId: string, projectId: string, envId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "projects", projectId, "environments", envId],
    queryFn: () =>
      apiFetch<Environment>(`/orgs/${orgId}/projects/${projectId}/environments/${envId}`),
    enabled: !!orgId && !!projectId && !!envId,
  });
}

export function useCreateEnvironment(orgId: string, projectId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; promotion_rank: number; requires_approval: boolean }) =>
      apiFetch<Environment>(`/orgs/${orgId}/projects/${projectId}/environments`, {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "projects", projectId, "environments"] }),
  });
}

export function useWorkspaces(orgId: string, projectId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "projects", projectId, "workspaces"],
    queryFn: () => apiFetch<ListResponse<Workspace>>(`/orgs/${orgId}/projects/${projectId}/workspaces`),
    enabled: !!orgId && !!projectId,
  });
}

export function useWorkspace(orgId: string, projectId: string, workspaceId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "projects", projectId, "workspaces", workspaceId],
    queryFn: () =>
      apiFetch<Workspace>(`/orgs/${orgId}/projects/${projectId}/workspaces/${workspaceId}`),
    enabled: !!orgId && !!projectId && !!workspaceId,
  });
}

export function useCreateWorkspace(orgId: string, projectId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; execution_engine: string; environment_id?: string | null }) =>
      apiFetch<Workspace>(`/orgs/${orgId}/projects/${projectId}/workspaces`, {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "projects", projectId, "workspaces"] }),
  });
}

// useDeleteWorkspace - a genuine hard delete (Workspace + its own Runs/
// Variables/RoleBindings). No guard on locked/active-Run state - the
// backend allows it unconditionally, per an explicit operator choice.
export function useDeleteWorkspace(orgId: string, projectId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (workspaceId: string) =>
      apiFetch<void>(`/orgs/${orgId}/projects/${projectId}/workspaces/${workspaceId}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "projects", projectId, "workspaces"] }),
  });
}

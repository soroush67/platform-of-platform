import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { apiFetch } from "../client";
import type { ListResponse, Organization, Project, Team } from "../types";

export function useOrganizations() {
  return useQuery({
    queryKey: ["orgs"],
    queryFn: () => apiFetch<ListResponse<Organization>>("/orgs"),
  });
}

export function useOrganization(orgId: string) {
  return useQuery({
    queryKey: ["orgs", orgId],
    queryFn: () => apiFetch<Organization>(`/orgs/${orgId}`),
    enabled: !!orgId,
  });
}

export function useCreateOrganization() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; slug: string }) =>
      apiFetch<Organization>("/orgs", { method: "POST", body: JSON.stringify(input) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs"] }),
  });
}

export function useArchiveOrganization(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => apiFetch<Organization>(`/orgs/${orgId}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["orgs"] });
      qc.invalidateQueries({ queryKey: ["orgs", orgId] });
    },
  });
}

export function useAddMember(orgId: string) {
  return useMutation({
    mutationFn: (userId: string) =>
      apiFetch<void>(`/orgs/${orgId}/members`, { method: "POST", body: JSON.stringify({ user_id: userId }) }),
  });
}

export function useChangeMemberRole(orgId: string) {
  return useMutation({
    mutationFn: ({ userId, role }: { userId: string; role: string }) =>
      apiFetch<void>(`/orgs/${orgId}/members/${userId}/role`, { method: "PUT", body: JSON.stringify({ role }) }),
  });
}

export function useProjects(orgId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "projects"],
    queryFn: () => apiFetch<ListResponse<Project>>(`/orgs/${orgId}/projects`),
    enabled: !!orgId,
  });
}

export function useProject(orgId: string, projectId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "projects", projectId],
    queryFn: () => apiFetch<Project>(`/orgs/${orgId}/projects/${projectId}`),
    enabled: !!orgId && !!projectId,
  });
}

export function useCreateProject(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; slug: string; description: string }) =>
      apiFetch<Project>(`/orgs/${orgId}/projects`, { method: "POST", body: JSON.stringify(input) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "projects"] }),
  });
}

export function useCreateTeam(orgId: string) {
  return useMutation({
    mutationFn: (name: string) =>
      apiFetch<Team>(`/orgs/${orgId}/teams`, { method: "POST", body: JSON.stringify({ name }) }),
  });
}

export function useAddTeamMember(orgId: string, teamId: string) {
  return useMutation({
    mutationFn: (userId: string) =>
      apiFetch<void>(`/orgs/${orgId}/teams/${teamId}/members`, {
        method: "POST",
        body: JSON.stringify({ user_id: userId }),
      }),
  });
}

export function useRemoveTeamMember(orgId: string, teamId: string) {
  return useMutation({
    mutationFn: (userId: string) =>
      apiFetch<void>(`/orgs/${orgId}/teams/${teamId}/members/${userId}`, { method: "DELETE" }),
  });
}

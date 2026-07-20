import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { apiFetch } from "../client";
import type { AvailableUser, ListResponse, Member, Organization, Project, Team, TeamMember } from "../types";

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

// useArchiveOrganizationById is useArchiveOrganization's own sibling for
// a page managing many organizations at once (PlatformAdminPage) rather
// than one bound at mount time - the id is a mutate-time argument here
// instead of a hook-construction one.
export function useArchiveOrganizationById() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (orgId: string) => apiFetch<Organization>(`/orgs/${orgId}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs"] }),
  });
}

// useDeleteOrganization hits POST /orgs/{id}/hard-delete - a genuinely
// irreversible hard delete (backend calls OrganizationRepository.Purge
// directly), NOT the soft-delete useArchiveOrganizationById above calls.
// No response body (204) - unlike Archive/Create, there's no updated
// Organization to hand back, the row is gone.
export function useDeleteOrganization() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (orgId: string) => apiFetch<void>(`/orgs/${orgId}/hard-delete`, { method: "POST" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs"] }),
  });
}

export function useMembers(orgId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "members"],
    queryFn: () => apiFetch<ListResponse<Member>>(`/orgs/${orgId}/members`),
    enabled: !!orgId,
  });
}

// useAvailableUsers backs the Members page's "add existing user" picker -
// every platform User not already a member of this org (soroush's own
// case: created once, never added to any org, invisible on every org's
// roster and un-creatable a second time since usernames are platform-
// global - this is how an admin gets them in without knowing their raw id).
export function useAvailableUsers(orgId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "members", "available"],
    queryFn: () => apiFetch<ListResponse<AvailableUser>>(`/orgs/${orgId}/members/available`),
    enabled: !!orgId,
  });
}

export function useAddMember(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (userId: string) =>
      apiFetch<void>(`/orgs/${orgId}/members`, { method: "POST", body: JSON.stringify({ user_id: userId }) }),
    // Prefix match (invalidateQueries' default) also catches
    // ["orgs", orgId, "members", "available"] - the same user must
    // disappear from the picker as it appears in the roster.
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "members"] }),
  });
}

export function useChangeMemberRole(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ userId, role }: { userId: string; role: string }) =>
      apiFetch<void>(`/orgs/${orgId}/members/${userId}/role`, { method: "PUT", body: JSON.stringify({ role }) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "members"] }),
  });
}

// useBlockMember/useUnblockMember - per-organization suspension only
// (see Member's own "blocked" field comment).
export function useBlockMember(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (userId: string) => apiFetch<void>(`/orgs/${orgId}/members/${userId}/block`, { method: "PUT" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "members"] }),
  });
}

export function useUnblockMember(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (userId: string) => apiFetch<void>(`/orgs/${orgId}/members/${userId}/unblock`, { method: "PUT" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "members"] }),
  });
}

// useRemoveMember - a real, permanent removal of this membership (the
// long-flagged gap, finally closed) - the User account itself is
// untouched.
export function useRemoveMember(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (userId: string) => apiFetch<void>(`/orgs/${orgId}/members/${userId}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "members"] }),
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

// useDeleteProject - a genuine hard delete (Project + everything scoped
// under it: Workspaces, Environments, Runs, Variables, RoleBindings,
// compose-file links). No response body (204).
export function useDeleteProject(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (projectId: string) =>
      apiFetch<void>(`/orgs/${orgId}/projects/${projectId}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "projects"] }),
  });
}

export function useTeams(orgId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "teams"],
    queryFn: () => apiFetch<ListResponse<Team>>(`/orgs/${orgId}/teams`),
    enabled: !!orgId,
  });
}

export function useCreateTeam(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) =>
      apiFetch<Team>(`/orgs/${orgId}/teams`, { method: "POST", body: JSON.stringify({ name }) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "teams"] }),
  });
}

// useUpdateTeam - a rename, nothing else (Team has no other mutable
// field).
export function useUpdateTeam(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ teamId, name }: { teamId: string; name: string }) =>
      apiFetch<Team>(`/orgs/${orgId}/teams/${teamId}`, { method: "PUT", body: JSON.stringify({ name }) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "teams"] }),
  });
}

// useDeleteTeam - a real, permanent removal: the Team, its own
// memberships, and every RoleBinding granted to it.
export function useDeleteTeam(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (teamId: string) => apiFetch<void>(`/orgs/${orgId}/teams/${teamId}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["orgs", orgId, "teams"] });
      qc.invalidateQueries({ queryKey: ["orgs", orgId, "role-bindings"] });
    },
  });
}

// useTeamMembers is the real per-Team roster TeamsPage.tsx was missing -
// without it, "Add member" had no visible confirmation and "Remove
// member" was a blind guess from the org-wide member list.
export function useTeamMembers(orgId: string, teamId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "teams", teamId, "members"],
    queryFn: () => apiFetch<ListResponse<TeamMember>>(`/orgs/${orgId}/teams/${teamId}/members`),
    enabled: !!orgId && !!teamId,
  });
}

export function useAddTeamMember(orgId: string, teamId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (userId: string) =>
      apiFetch<void>(`/orgs/${orgId}/teams/${teamId}/members`, {
        method: "POST",
        body: JSON.stringify({ user_id: userId }),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "teams", teamId, "members"] }),
  });
}

export function useRemoveTeamMember(orgId: string, teamId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (userId: string) =>
      apiFetch<void>(`/orgs/${orgId}/teams/${teamId}/members/${userId}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "teams", teamId, "members"] }),
  });
}

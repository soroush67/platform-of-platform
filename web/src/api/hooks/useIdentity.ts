import { useMutation } from "@tanstack/react-query";

import { apiFetch } from "../client";
import type { ApiKeyCreateResponse, ServiceAccount, User } from "../types";

// useCreateUser calls POST /users - org-independent by design (User is
// platform-global, internal/identity/domain), matching that route's own
// shape. The admin-panel "create user & add to this org" flow
// (MembersPage) is what supplies the org context, one call later.
export function useCreateUser() {
  return useMutation({
    mutationFn: (input: { username: string; email: string; password: string }) =>
      apiFetch<User>("/users", {
        method: "POST",
        body: JSON.stringify({ ...input, auth_source: "local" }),
      }),
  });
}

export function useCreateServiceAccount(orgId: string) {
  return useMutation({
    mutationFn: (input: { name: string; description: string }) =>
      apiFetch<ServiceAccount>(`/orgs/${orgId}/service-accounts`, {
        method: "POST",
        body: JSON.stringify(input),
      }),
  });
}

export function useCreateApiKey(orgId: string, serviceAccountId: string) {
  return useMutation({
    mutationFn: (input: { name: string; scopes: string[]; expires_at?: string | null }) =>
      apiFetch<ApiKeyCreateResponse>(`/orgs/${orgId}/service-accounts/${serviceAccountId}/api-keys`, {
        method: "POST",
        body: JSON.stringify(input),
      }),
  });
}

export function useRevokeApiKey(orgId: string, serviceAccountId: string) {
  return useMutation({
    mutationFn: (keyId: string) =>
      apiFetch<void>(`/orgs/${orgId}/service-accounts/${serviceAccountId}/api-keys/${keyId}`, {
        method: "DELETE",
      }),
  });
}

import { useMutation } from "@tanstack/react-query";

import { apiFetch } from "../client";
import type { ApiKeyCreateResponse, ServiceAccount } from "../types";

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

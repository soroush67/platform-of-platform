import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { apiFetch } from "../client";
import type { ListResponse, SecretMount } from "../types";

export function useSecretMounts(orgId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "secret-mounts"],
    queryFn: () => apiFetch<ListResponse<SecretMount>>(`/orgs/${orgId}/secret-mounts`),
    enabled: !!orgId,
  });
}

export function useCreateSecretMount(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; backend_type: string; address: string; role_id: string; secret_id: string }) =>
      apiFetch<SecretMount>(`/orgs/${orgId}/secret-mounts`, { method: "POST", body: JSON.stringify(input) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "secret-mounts"] }),
  });
}

export function useTestSecretMountConnection(orgId: string) {
  return useMutation({
    mutationFn: (mountId: string) =>
      apiFetch<void>(`/orgs/${orgId}/secret-mounts/${mountId}/test-connection`, { method: "POST" }),
  });
}

// useWriteSecret stores a value directly into a Secret Mount's backing
// Vault at a given path - lets a form (e.g. Fleet's Add Machine) collect
// a credential inline instead of requiring an out-of-band `vault kv put`
// first. KV v2 writes are upserts, so calling this more than once with
// the same path/value (e.g. from both a "Test connection" and a
// "Create" click) is always safe.
export function useWriteSecret(orgId: string) {
  return useMutation({
    mutationFn: (input: { mountId: string; path: string; value: string }) =>
      apiFetch<void>(`/orgs/${orgId}/secret-mounts/${input.mountId}/secrets`, {
        method: "POST",
        body: JSON.stringify({ path: input.path, value: input.value }),
      }),
  });
}

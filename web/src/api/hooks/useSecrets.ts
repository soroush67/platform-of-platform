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

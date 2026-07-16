import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { apiFetch, apiFetchRaw } from "../client";
import {
  isTerminalOperationStatus,
  type ComposeFile,
  type CredentialType,
  type FleetNetwork,
  type FleetVariable,
  type FleetVolume,
  type ListResponse,
  type Machine,
  type Operation,
  type OperationType,
  type VarType,
  type VolumeAttachment,
} from "../types";

// ---- Machines ----

export function useMachines(orgId: string, includeArchived = false) {
  return useQuery({
    queryKey: ["orgs", orgId, "machines", { includeArchived }],
    queryFn: () => apiFetch<ListResponse<Machine>>(`/orgs/${orgId}/machines?include_archived=${includeArchived}`),
    enabled: !!orgId,
  });
}

export function useMachine(orgId: string, machineId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "machines", machineId],
    queryFn: () => apiFetch<Machine>(`/orgs/${orgId}/machines/${machineId}`),
    enabled: !!orgId && !!machineId,
  });
}

export interface CreateMachineInput {
  name: string;
  host: string;
  ssh_port: number;
  ssh_user: string;
  credential_type: CredentialType;
  credential_mount_id: string;
  credential_path: string;
  deploy_base_path: string;
}

export function useCreateMachine(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateMachineInput) =>
      apiFetch<Machine>(`/orgs/${orgId}/machines`, { method: "POST", body: JSON.stringify(input) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "machines"] }),
  });
}

export interface UpdateMachineInput {
  ssh_user?: string;
  credential_type?: CredentialType;
  credential_mount_id?: string;
  credential_path?: string;
  deploy_base_path?: string;
}

export function useUpdateMachine(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ machineId, ...input }: UpdateMachineInput & { machineId: string }) =>
      apiFetch<Machine>(`/orgs/${orgId}/machines/${machineId}`, { method: "PATCH", body: JSON.stringify(input) }),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: ["orgs", orgId, "machines"] });
      qc.invalidateQueries({ queryKey: ["orgs", orgId, "machines", vars.machineId] });
    },
  });
}

// useArchiveMachine hits DELETE, which is really "delete-or-archive" -
// ArchiveMachineHandler's own {archived: bool} response tells the caller
// which outcome actually happened (a Machine with real Operation history
// can never be hard-deleted, matching the ported Python original).
export function useArchiveMachine(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (machineId: string) =>
      apiFetch<{ archived: boolean }>(`/orgs/${orgId}/machines/${machineId}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "machines"] }),
  });
}

export interface TestMachineConnectionInput {
  host: string;
  ssh_port: number;
  ssh_user: string;
  credential_type: CredentialType;
  credential_mount_id: string;
  credential_path: string;
}

// useTestMachineConnection probes WITHOUT a saved Machine row - lets an
// operator verify a host/credential pair before committing to Create.
export function useTestMachineConnection(orgId: string) {
  return useMutation({
    mutationFn: (input: TestMachineConnectionInput) =>
      apiFetch<{ connection_status: string; docker_status: string }>(`/orgs/${orgId}/machines/test-connection`, {
        method: "POST",
        body: JSON.stringify(input),
      }),
  });
}

// useCheckMachineConnection is the saved-Machine counterpart - probes
// AND persists the result (Machine.connection_status/docker_status).
export function useCheckMachineConnection(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (machineId: string) =>
      apiFetch<Machine>(`/orgs/${orgId}/machines/${machineId}/check-connection`, { method: "POST" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "machines"] }),
  });
}

// ---- Networks ----

export function useFleetNetworks(orgId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "networks"],
    queryFn: () => apiFetch<ListResponse<FleetNetwork>>(`/orgs/${orgId}/networks`),
    enabled: !!orgId,
  });
}

export function useCreateFleetNetwork(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; external: boolean }) =>
      apiFetch<FleetNetwork>(`/orgs/${orgId}/networks`, { method: "POST", body: JSON.stringify(input) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "networks"] }),
  });
}

export function useDeleteFleetNetwork(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (networkId: string) => apiFetch<void>(`/orgs/${orgId}/networks/${networkId}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "networks"] }),
  });
}

// ---- Volumes ----

export function useFleetVolumes(orgId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "volumes"],
    queryFn: () => apiFetch<ListResponse<FleetVolume>>(`/orgs/${orgId}/volumes`),
    enabled: !!orgId,
  });
}

export function useCreateFleetVolume(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; host_path: string }) =>
      apiFetch<FleetVolume>(`/orgs/${orgId}/volumes`, { method: "POST", body: JSON.stringify(input) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "volumes"] }),
  });
}

export function useDeleteFleetVolume(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (volumeId: string) => apiFetch<void>(`/orgs/${orgId}/volumes/${volumeId}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "volumes"] }),
  });
}

// ---- Compose files ----

export function useComposeFiles(orgId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "compose-files"],
    queryFn: () => apiFetch<ListResponse<ComposeFile>>(`/orgs/${orgId}/compose-files`),
    enabled: !!orgId,
  });
}

export function useComposeFile(orgId: string, composeFileId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "compose-files", composeFileId],
    queryFn: () => apiFetch<ComposeFile>(`/orgs/${orgId}/compose-files/${composeFileId}`),
    enabled: !!orgId && !!composeFileId,
  });
}

export function useCreateComposeFile(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; compose_content: string; is_global: boolean }) =>
      apiFetch<ComposeFile>(`/orgs/${orgId}/compose-files`, { method: "POST", body: JSON.stringify(input) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "compose-files"] }),
  });
}

export function useUpdateComposeFileContent(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ composeFileId, composeContent }: { composeFileId: string; composeContent: string }) =>
      apiFetch<void>(`/orgs/${orgId}/compose-files/${composeFileId}/content`, {
        method: "PUT",
        body: JSON.stringify({ compose_content: composeContent }),
      }),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: ["orgs", orgId, "compose-files"] });
      qc.invalidateQueries({ queryKey: ["orgs", orgId, "compose-files", vars.composeFileId] });
    },
  });
}

// ---- Network/volume attachments ----

export function useComposeFileNetworks(orgId: string, composeFileId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "compose-files", composeFileId, "networks"],
    queryFn: () =>
      apiFetch<ListResponse<FleetNetwork>>(`/orgs/${orgId}/compose-files/${composeFileId}/networks`),
    enabled: !!orgId && !!composeFileId,
  });
}

export function useAttachNetwork(orgId: string, composeFileId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (networkId: string) =>
      apiFetch<void>(`/orgs/${orgId}/compose-files/${composeFileId}/networks`, {
        method: "POST",
        body: JSON.stringify({ network_id: networkId }),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "compose-files", composeFileId, "networks"] }),
  });
}

export function useDetachNetwork(orgId: string, composeFileId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (networkId: string) =>
      apiFetch<void>(`/orgs/${orgId}/compose-files/${composeFileId}/networks/${networkId}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "compose-files", composeFileId, "networks"] }),
  });
}

export function useComposeFileVolumes(orgId: string, composeFileId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "compose-files", composeFileId, "volumes"],
    queryFn: () =>
      apiFetch<ListResponse<VolumeAttachment>>(`/orgs/${orgId}/compose-files/${composeFileId}/volumes`),
    enabled: !!orgId && !!composeFileId,
  });
}

export function useAttachVolume(orgId: string, composeFileId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ volumeId, containerPath }: { volumeId: string; containerPath: string }) =>
      apiFetch<void>(`/orgs/${orgId}/compose-files/${composeFileId}/volumes`, {
        method: "POST",
        body: JSON.stringify({ volume_id: volumeId, container_path: containerPath }),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "compose-files", composeFileId, "volumes"] }),
  });
}

export function useDetachVolume(orgId: string, composeFileId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (volumeId: string) =>
      apiFetch<void>(`/orgs/${orgId}/compose-files/${composeFileId}/volumes/${volumeId}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "compose-files", composeFileId, "volumes"] }),
  });
}

// ---- Fleet variables ----

export function useFleetVariables(orgId: string, composeFileId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "compose-files", composeFileId, "variables"],
    queryFn: () =>
      apiFetch<ListResponse<FleetVariable>>(`/orgs/${orgId}/compose-files/${composeFileId}/variables`),
    enabled: !!orgId && !!composeFileId,
  });
}

export interface CreateFleetVariableInput {
  key: string;
  var_type: VarType;
  value?: string;
  secret_mount_id?: string;
  secret_path?: string;
  file_target_path?: string;
}

export function useCreateFleetVariable(orgId: string, composeFileId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateFleetVariableInput) =>
      apiFetch<FleetVariable>(`/orgs/${orgId}/compose-files/${composeFileId}/variables`, {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "compose-files", composeFileId, "variables"] }),
  });
}

export interface UpdateFleetVariableInput {
  value?: string;
  secret_mount_id?: string;
  secret_path?: string;
  file_target_path?: string;
}

export function useUpdateFleetVariable(orgId: string, composeFileId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ variableId, ...input }: UpdateFleetVariableInput & { variableId: string }) =>
      apiFetch<FleetVariable>(`/orgs/${orgId}/compose-files/${composeFileId}/variables/${variableId}`, {
        method: "PUT",
        body: JSON.stringify(input),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "compose-files", composeFileId, "variables"] }),
  });
}

export function useDeleteFleetVariable(orgId: string, composeFileId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (variableId: string) =>
      apiFetch<void>(`/orgs/${orgId}/compose-files/${composeFileId}/variables/${variableId}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "compose-files", composeFileId, "variables"] }),
  });
}

// ---- Operations ----

export function useOperations(orgId: string, composeFileId?: string, machineId?: string) {
  const params = new URLSearchParams();
  if (composeFileId) params.set("compose_file_id", composeFileId);
  if (machineId) params.set("machine_id", machineId);
  const qs = params.toString();
  return useQuery({
    queryKey: ["orgs", orgId, "operations", { composeFileId, machineId }],
    queryFn: () => apiFetch<ListResponse<Operation>>(`/orgs/${orgId}/operations${qs ? `?${qs}` : ""}`),
    enabled: !!orgId,
    refetchInterval: 10000,
  });
}

// useOperation polls every 2s while the operation is non-terminal, same
// self-stopping shape as Execution's own useRun.
export function useOperation(orgId: string, operationId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "operations", operationId],
    queryFn: () => apiFetch<Operation>(`/orgs/${orgId}/operations/${operationId}`),
    enabled: !!orgId && !!operationId,
    refetchInterval: (query) => {
      const op = query.state.data as Operation | undefined;
      if (!op || !isTerminalOperationStatus(op.status)) return 2000;
      return false;
    },
  });
}

export interface TriggerOperationInput {
  compose_file_id: string;
  machine_id: string;
  operation_type: OperationType;
}

export function useTriggerOperation(orgId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: TriggerOperationInput) =>
      apiFetch<Operation>(`/orgs/${orgId}/operations`, {
        method: "POST",
        headers: { "Idempotency-Key": crypto.randomUUID() },
        body: JSON.stringify(input),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "operations"] }),
  });
}

// ---- Live log stream ----
//
// useOperationLogStream is deliberately NOT a react-query hook - it's a
// long-lived Server-Sent-Events subscription
// (GET .../operations/{id}/stream), not a cacheable request/response.
// Not the browser's native EventSource either (can't set the
// Authorization header without pushing the access token into a URL
// query param) - fetch() + ReadableStream, hand-parsing the same
// "data: <line>\n\n" / "event: done\ndata: {...}\n\n" wire format
// StreamOperationHandler writes (internal/fleet/adapters/http/
// stream_handler.go).

export interface OperationLogStreamState {
  lines: string[];
  done: boolean;
  exitCode: number | null;
  error: string | null;
}

export function useOperationLogStream(orgId: string, operationId: string, enabled: boolean): OperationLogStreamState {
  const [state, setState] = useState<OperationLogStreamState>({ lines: [], done: false, exitCode: null, error: null });

  useEffect(() => {
    if (!enabled || !orgId || !operationId) return;
    setState({ lines: [], done: false, exitCode: null, error: null });
    const controller = new AbortController();

    async function consume() {
      try {
        const res = await apiFetchRaw(`/orgs/${orgId}/operations/${operationId}/stream`, {
          signal: controller.signal,
        });
        if (!res.ok || !res.body) {
          throw new Error(`stream request failed (${res.status})`);
        }
        const reader = res.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";

        for (;;) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });

          const frames = buffer.split("\n\n");
          buffer = frames.pop() ?? "";

          for (const frame of frames) {
            let event = "message";
            let data = "";
            for (const line of frame.split("\n")) {
              if (line.startsWith("event: ")) event = line.slice("event: ".length);
              else if (line.startsWith("data: ")) data = line.slice("data: ".length);
            }
            if (event === "done") {
              const exitCode = data ? (JSON.parse(data).exit_code ?? null) : null;
              setState((prev) => ({ ...prev, done: true, exitCode }));
            } else {
              setState((prev) => ({ ...prev, lines: [...prev.lines, data] }));
            }
          }
        }
      } catch (err) {
        if (controller.signal.aborted) return;
        setState((prev) => ({ ...prev, error: err instanceof Error ? err.message : "stream failed" }));
      }
    }

    void consume();
    return () => controller.abort();
  }, [orgId, operationId, enabled]);

  return state;
}

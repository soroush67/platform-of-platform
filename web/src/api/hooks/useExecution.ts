import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { apiFetch } from "../client";
import { isTerminalRunStatus, type ListResponse, type Run } from "../types";

function runsBase(orgId: string, projectId: string, workspaceId: string) {
  return `/orgs/${orgId}/projects/${projectId}/workspaces/${workspaceId}/runs`;
}

// useRuns polls at a fixed, longer interval purely so a freshly-triggered
// run's row updates without a full page reload - no WebSocket gateway
// exists (deliberate simplification, see docs/architecture/16-web-ui.md
// vs. what's actually built).
export function useRuns(orgId: string, projectId: string, workspaceId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "workspaces", workspaceId, "runs"],
    queryFn: () => apiFetch<ListResponse<Run>>(runsBase(orgId, projectId, workspaceId)),
    enabled: !!orgId && !!projectId && !!workspaceId,
    refetchInterval: 10000,
  });
}

// useRun polls every 2s while the run is non-terminal and stops itself
// the instant a terminal status is fetched - the one piece of domain
// logic (RunStatus.IsTerminal()) mirrored client-side.
export function useRun(orgId: string, projectId: string, workspaceId: string, runId: string) {
  return useQuery({
    queryKey: ["orgs", orgId, "workspaces", workspaceId, "runs", runId],
    queryFn: () => apiFetch<Run>(`${runsBase(orgId, projectId, workspaceId)}/${runId}`),
    enabled: !!orgId && !!projectId && !!workspaceId && !!runId,
    refetchInterval: (query) => {
      const run = query.state.data as Run | undefined;
      if (!run || !isTerminalRunStatus(run.status)) return 2000;
      return false;
    },
  });
}

export function useTriggerRun(orgId: string, projectId: string, workspaceId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (idempotencyKey?: string) =>
      apiFetch<Run>(runsBase(orgId, projectId, workspaceId), {
        method: "POST",
        headers: idempotencyKey ? { "Idempotency-Key": idempotencyKey } : undefined,
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["orgs", orgId, "workspaces", workspaceId, "runs"] }),
  });
}

export function useCancelRun(orgId: string, projectId: string, workspaceId: string, runId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      apiFetch<Run>(`${runsBase(orgId, projectId, workspaceId)}/${runId}/cancel`, { method: "POST" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["orgs", orgId, "workspaces", workspaceId, "runs", runId] });
      qc.invalidateQueries({ queryKey: ["orgs", orgId, "workspaces", workspaceId, "runs"] });
    },
  });
}

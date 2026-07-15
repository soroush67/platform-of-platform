import { useInfiniteQuery } from "@tanstack/react-query";

import { apiFetch } from "../client";
import type { AuditLogPage } from "../types";

export function useAuditLog(orgId: string) {
  return useInfiniteQuery({
    queryKey: ["orgs", orgId, "audit-log"],
    queryFn: ({ pageParam }: { pageParam?: string }) => {
      const qs = pageParam ? `?cursor=${encodeURIComponent(pageParam)}` : "";
      return apiFetch<AuditLogPage>(`/orgs/${orgId}/audit-log${qs}`);
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) => lastPage.next_cursor,
    enabled: !!orgId,
  });
}

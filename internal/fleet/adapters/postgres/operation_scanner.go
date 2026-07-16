package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/fleet/application"
)

// OperationScanner is root-pool-backed (bypasses RLS), NOT the same
// pool OperationRepository above uses - operations is tenant-facing
// (unlike outbox_events, which deliberately has no RLS for exactly this
// reason), so a genuine cross-org scan for queued work needs the same
// root-connection exception internal/platform/idempotency.Reaper
// already established for the identical problem shape. DeployExecutor
// uses this to discover candidates, then OperationRepository.TryClaim
// (app-pool, RLS-scoped, set_config'd per org) to actually take
// ownership of one - the same two-tier discover-then-claim split
// StaleRunReaperService already proved out.
type OperationScanner struct {
	rootPool *pgxpool.Pool
}

func NewOperationScanner(rootPool *pgxpool.Pool) *OperationScanner {
	return &OperationScanner{rootPool: rootPool}
}

func (s *OperationScanner) FindQueuedCandidates(ctx context.Context, limit int) ([]application.OperationCandidate, error) {
	rows, err := s.rootPool.Query(ctx,
		`SELECT id, organization_id FROM operations WHERE status = 'queued' ORDER BY created_at LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []application.OperationCandidate
	for rows.Next() {
		var c application.OperationCandidate
		if err := rows.Scan(&c.OperationID, &c.OrganizationID); err != nil {
			return nil, err
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return candidates, nil
}

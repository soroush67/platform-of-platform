package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"

	"platform-of-platform/internal/tenancy/domain"
)

// RootMembershipRepository backs application.RootMembershipRepository -
// see that port's own doc comment for the full RLS/root reasoning. It is
// constructed with the root connection (cfg.DatabaseURL, the same role
// migrations run as - see cmd/control-plane/main.go's own rootPool
// comment), NOT the normal RLS-constrained platform_app pool every other
// repository in this package uses. This type exists ONLY for this one
// cross-org read - every other Tenancy write/read still goes through
// OrganizationRepository/MembershipRepository above, unaffected.
type RootMembershipRepository struct {
	rootPool *pgxpool.Pool
}

func NewRootMembershipRepository(rootPool *pgxpool.Pool) *RootMembershipRepository {
	return &RootMembershipRepository{rootPool: rootPool}
}

// ListOrganizationsForUser joins organization_memberships to
// organizations directly against rootPool - root bypasses RLS (the same
// property idempotency.Reaper's own doc comment already relies on), so
// no set_config call is needed or even possible here (there's no single
// org_id to scope to for "every org this user belongs to"). Archived
// organizations are deliberately included, not filtered - a member
// should be able to see an org they belong to went archived (e.g. to
// understand why writes there now 409), not have it silently vanish
// from their own list.
func (r *RootMembershipRepository) ListOrganizationsForUser(ctx context.Context, userID string) ([]*domain.Organization, error) {
	rows, err := r.rootPool.Query(ctx,
		`SELECT o.id, o.name, o.slug, o.settings, o.quota, o.status, o.archived_at, o.created_at
		 FROM organizations o
		 JOIN organization_memberships m ON m.organization_id = o.id
		 WHERE m.user_id = $1
		 ORDER BY o.created_at`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orgs []*domain.Organization
	for rows.Next() {
		var org domain.Organization
		var settings, quota []byte
		if err := rows.Scan(&org.ID, &org.Name, &org.Slug, &settings, &quota, &org.Status, &org.ArchivedAt, &org.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(settings, &org.Settings); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(quota, &org.Quota); err != nil {
			return nil, err
		}
		orgs = append(orgs, &org)
	}
	return orgs, rows.Err()
}

// CountOrganizations - a genuine cross-org COUNT(*) against rootPool
// (organizations has FORCE ROW LEVEL SECURITY, so the normal app pool
// would silently report 0 always with no app.current_org_id set - the
// same reasoning ListOrganizationsForUser's own doc comment already
// gives). Backs CreateOrganizationService's first-org-ever bootstrap
// check.
func (r *RootMembershipRepository) CountOrganizations(ctx context.Context) (int, error) {
	var count int
	err := r.rootPool.QueryRow(ctx, `SELECT count(*) FROM organizations`).Scan(&count)
	return count, err
}

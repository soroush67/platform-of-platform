package application

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"platform-of-platform/internal/audit/domain"
)

const permissionOrganizationManage = "organization:manage"

// defaultPageLimit/maxPageLimit - a real cap the client can't override
// past (docs/architecture/04-api-design.md style bound), same "a client
// can ask for less, never for unbounded" posture pagination exists to
// enforce in the first place - an audit log is exactly the table this
// operator flagged as "genuinely problematic at high volume" without it.
const (
	defaultPageLimit = 50
	maxPageLimit     = 500
)

// ListAuditEntriesService implements `GET /api/v1/orgs/{org}/audit-log`
// (docs/architecture/04-api-design.md §1's own named route). Gated by
// organization:manage, not mere membership - an audit trail is
// sensitive by nature (who did what, when), a stricter bar than the
// membership-only gate every other read in this codebase uses.
//
// Cursor-based (keyset), not OFFSET-based: an OFFSET-based page N+1
// query still has to skip N*limit rows server-side every single time,
// which is exactly the "really problematic at high volume" failure mode
// named for this endpoint - a keyset cursor (created_at, id) turns every
// page into the same cheap indexed range scan regardless of how deep
// the caller has paged.
type ListAuditEntriesService struct {
	repo        AuditEntryRepository
	permChecker PermissionChecker
}

func NewListAuditEntriesService(repo AuditEntryRepository, permChecker PermissionChecker) *ListAuditEntriesService {
	return &ListAuditEntriesService{repo: repo, permChecker: permChecker}
}

type ListAuditEntriesInput struct {
	OrganizationID   string
	RequestingUserID string
	Limit            int
	// Cursor is the opaque string EncodeCursor produced for the last
	// entry of the *previous* page - "" for the first page.
	Cursor string
}

type EntryPage struct {
	Entries []*domain.Entry
	// NextCursor is "" when this page was the last one - the HTTP
	// adapter omits the field entirely in that case, the same "absent,
	// not null" convention this codebase already uses for optional
	// response fields (e.g. Run.FinishedAt).
	NextCursor string
}

func (s *ListAuditEntriesService) Execute(ctx context.Context, in ListAuditEntriesInput) (*EntryPage, error) {
	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionOrganizationManage)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	limit := in.Limit
	if limit <= 0 {
		limit = defaultPageLimit
	}
	if limit > maxPageLimit {
		limit = maxPageLimit
	}

	var beforeCreatedAt *time.Time
	var beforeID *string
	if in.Cursor != "" {
		t, id, err := DecodeCursor(in.Cursor)
		if err != nil {
			return nil, &domain.ValidationError{Message: "invalid cursor"}
		}
		beforeCreatedAt, beforeID = &t, &id
	}

	// Fetch one extra row - cheap, standard keyset-pagination trick to
	// learn "is there a next page" without a second COUNT(*) query
	// (which would reintroduce the exact full-table-scan cost this
	// pagination scheme exists to avoid).
	entries, err := s.repo.ListByOrganization(ctx, in.OrganizationID, limit+1, beforeCreatedAt, beforeID)
	if err != nil {
		return nil, err
	}

	page := &EntryPage{Entries: entries}
	if len(entries) > limit {
		last := entries[limit-1]
		page.Entries = entries[:limit]
		page.NextCursor = EncodeCursor(last.CreatedAt, last.ID)
	}

	return page, nil
}

// EncodeCursor/DecodeCursor - an opaque, base64-encoded
// "RFC3339Nano|id" pair, not a bare timestamp+id the client could
// construct or tamper with meaningfully (it doesn't need to be
// cryptographically opaque - there's nothing sensitive in a
// created_at+id pair - just self-contained and versionable, matching
// the "opaque cursor" convention every real paginated API uses instead
// of leaking OFFSET as an API contract).
func EncodeCursor(createdAt time.Time, id string) string {
	raw := fmt.Sprintf("%s|%s", createdAt.Format(time.RFC3339Nano), id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func DecodeCursor(cursor string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", fmt.Errorf("malformed cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", err
	}
	return t, parts[1], nil
}

package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// GetOrganizationService implements `GET /api/v1/orgs/{id}`
// (docs/architecture/04-api-design.md §1). Like org creation, this is
// deliberately unauthenticated for now - the id in the URL is taken as
// the org to scope RLS to, rather than derived from an authenticated
// Principal's own org membership (Stage 4 §4). That means this endpoint
// doesn't yet prove cross-tenant isolation on its own (a caller who
// knows any org's id can read its name/slug) - it proves the SELECT-side
// RLS wiring works for real, which Create's INSERT-side test didn't
// cover. Real cross-tenant read isolation needs the Identity/RBAC auth
// middleware (a later slice) resolving the session variable from who's
// asking, not from what they typed in the URL.
type GetOrganizationService struct {
	repo OrganizationRepository
}

func NewGetOrganizationService(repo OrganizationRepository) *GetOrganizationService {
	return &GetOrganizationService{repo: repo}
}

func (s *GetOrganizationService) Execute(ctx context.Context, id string) (*domain.Organization, error) {
	return s.repo.GetByID(ctx, id)
}

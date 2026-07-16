package application

import (
	"context"
	"time"

	"platform-of-platform/internal/tenancy/domain"
)

// MemberSummary is a read-model composed from three bounded contexts
// (Tenancy's own membership row, Identity's user record, RBAC's
// org-scope role_binding) - it lives here, not in tenancy/domain, since
// it isn't a domain aggregate, just what the member roster needs to
// display. RoleName is "" when the member has no org-scope binding
// (RoleReader.GetOrgScopeRoleName's own found=false case) - not an
// error, a real and displayable state.
type MemberSummary struct {
	UserID   string
	Username string
	Email    string
	RoleName string
	JoinedAt time.Time
}

// ListMembersService implements `GET /api/v1/orgs/{org}/members` - same
// membership-only gate as ListProjectsService (any member can see the
// roster, no organization:manage requirement - that's AddMember/
// ChangeMemberRole's own, stricter gate for actually changing it).
type ListMembersService struct {
	membershipRepo MembershipRepository
	userReader     UserReader
	roleReader     RoleReader
}

func NewListMembersService(membershipRepo MembershipRepository, userReader UserReader, roleReader RoleReader) *ListMembersService {
	return &ListMembersService{membershipRepo: membershipRepo, userReader: userReader, roleReader: roleReader}
}

func (s *ListMembersService) Execute(ctx context.Context, organizationID, requestingUserID string) ([]MemberSummary, error) {
	isMember, err := s.membershipRepo.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrOrganizationNotFound
	}

	memberships, err := s.membershipRepo.ListByOrganization(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	summaries := make([]MemberSummary, 0, len(memberships))
	for _, m := range memberships {
		// found=false for either resolution just means an emptier row,
		// not a failed roster - a user record or role binding that's
		// since disappeared shouldn't take the whole list down with it.
		username, email, _, err := s.userReader.GetUser(ctx, m.UserID)
		if err != nil {
			return nil, err
		}
		roleName, _, err := s.roleReader.GetOrgScopeRoleName(ctx, organizationID, m.UserID)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, MemberSummary{
			UserID:   m.UserID,
			Username: username,
			Email:    email,
			RoleName: roleName,
			JoinedAt: m.JoinedAt,
		})
	}

	return summaries, nil
}

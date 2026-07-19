package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// AvailableUser is what ListAvailableUsersService returns for a single
// candidate - a platform User not yet a member of this org.
type AvailableUser struct {
	ID       string
	Username string
	Email    string
}

// ListAvailableUsersService implements
// `GET /api/v1/orgs/{id}/members/available` - same organization:manage
// gate as AddMemberService (this is the picker AddMember's own UI
// needs), returns every platform User not already a member of this
// org. Exists because User creation is platform-global (Identity has
// no org concept) - a User created earlier (in this org, a different
// org, or via a "create user & add" attempt whose add-to-org step
// failed partway through) previously had no way back into an org's
// Members page except knowing their raw id by heart.
type ListAvailableUsersService struct {
	membershipRepo MembershipRepository
	permChecker    PermissionChecker
	userReader     UserReader
}

func NewListAvailableUsersService(membershipRepo MembershipRepository, permChecker PermissionChecker, userReader UserReader) *ListAvailableUsersService {
	return &ListAvailableUsersService{membershipRepo: membershipRepo, permChecker: permChecker, userReader: userReader}
}

func (s *ListAvailableUsersService) Execute(ctx context.Context, organizationID, requestingUserID string) ([]AvailableUser, error) {
	isMember, err := s.membershipRepo.IsMember(ctx, organizationID, requestingUserID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, domain.ErrOrganizationNotFound
	}

	allowed, err := s.permChecker.HasPermission(ctx, organizationID, requestingUserID, permissionOrganizationManage)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, domain.ErrForbidden
	}

	allUsers, err := s.userReader.ListAll(ctx)
	if err != nil {
		return nil, err
	}

	memberships, err := s.membershipRepo.ListByOrganization(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	existing := make(map[string]bool, len(memberships))
	for _, m := range memberships {
		existing[m.UserID] = true
	}

	available := make([]AvailableUser, 0, len(allUsers))
	for _, u := range allUsers {
		if !existing[u.ID] {
			available = append(available, AvailableUser{ID: u.ID, Username: u.Username, Email: u.Email})
		}
	}
	return available, nil
}

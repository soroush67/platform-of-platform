package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// subjectTypeUser mirrors delete_team.go's own subjectTypeTeam constant -
// the literal string rbac/domain.SubjectTypeUser itself would use,
// duplicated here since Tenancy never imports internal/rbac/domain.
const subjectTypeUser = "user"

// RemoveMemberInput implements `DELETE /orgs/{org}/members/{userID}` -
// the long-flagged gap this codebase never closed until now: a real,
// permanent removal of this User's membership in this org (operator's
// own scoped "Delete" meaning - the User account itself, and their
// membership in any other org, is untouched). Same organization:manage
// gate as ChangeMemberRole/BlockMember.
type RemoveMemberInput struct {
	OrganizationID   string
	RequestingUserID string
	TargetUserID     string
}

type RemoveMemberService struct {
	membershipRepo     MembershipRepository
	permChecker        PermissionChecker
	roleBindingCleaner RoleBindingCleaner
}

func NewRemoveMemberService(membershipRepo MembershipRepository, permChecker PermissionChecker, roleBindingCleaner RoleBindingCleaner) *RemoveMemberService {
	return &RemoveMemberService{membershipRepo: membershipRepo, permChecker: permChecker, roleBindingCleaner: roleBindingCleaner}
}

func (s *RemoveMemberService) Execute(ctx context.Context, in RemoveMemberInput) error {
	isRequesterMember, err := s.membershipRepo.IsMember(ctx, in.OrganizationID, in.RequestingUserID)
	if err != nil {
		return err
	}
	if !isRequesterMember {
		return domain.ErrOrganizationNotFound
	}

	allowed, err := s.permChecker.HasPermission(ctx, in.OrganizationID, in.RequestingUserID, permissionOrganizationManage)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}

	// MembershipExists, not IsMember - removing an already-blocked
	// member must still work.
	targetExists, err := s.membershipRepo.MembershipExists(ctx, in.OrganizationID, in.TargetUserID)
	if err != nil {
		return err
	}
	if !targetExists {
		return domain.ErrOrganizationNotFound
	}

	// RoleBindings cleaned up first - same "cross-context cleanup before
	// the local row is gone" ordering DeleteTeamService already
	// establishes (delete_team.go's own comment) - a removed member's
	// own grants would otherwise dangle, pointing at a user no longer in
	// this org.
	if err := s.roleBindingCleaner.DeleteForSubject(ctx, in.OrganizationID, subjectTypeUser, in.TargetUserID); err != nil {
		return err
	}

	return s.membershipRepo.Delete(ctx, in.OrganizationID, in.TargetUserID)
}

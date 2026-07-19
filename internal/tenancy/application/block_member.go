package application

import (
	"context"

	"platform-of-platform/internal/tenancy/domain"
)

// BlockMemberInput implements `PUT /orgs/{org}/members/{userID}/block` -
// operator's own explicitly scoped meaning: suspends this member's
// access to *this* organization only (MembershipRepository.IsMember's
// own doc comment explains the mechanism - blocked_at IS NULL is now
// part of every membership check everywhere). They stay a real platform
// User and keep working in any other org they belong to - this is
// deliberately not a platform-wide account suspension. Same
// organization:manage gate as ChangeMemberRole.
type BlockMemberInput struct {
	OrganizationID   string
	RequestingUserID string
	TargetUserID     string
}

type BlockMemberService struct {
	membershipRepo MembershipRepository
	permChecker    PermissionChecker
}

func NewBlockMemberService(membershipRepo MembershipRepository, permChecker PermissionChecker) *BlockMemberService {
	return &BlockMemberService{membershipRepo: membershipRepo, permChecker: permChecker}
}

func (s *BlockMemberService) Execute(ctx context.Context, in BlockMemberInput) error {
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

	// MembershipExists, not IsMember - the target must have a real
	// membership row, but may already be blocked (re-blocking is a
	// harmless no-op, not an error).
	targetExists, err := s.membershipRepo.MembershipExists(ctx, in.OrganizationID, in.TargetUserID)
	if err != nil {
		return err
	}
	if !targetExists {
		return domain.ErrOrganizationNotFound
	}

	return s.membershipRepo.SetBlocked(ctx, in.OrganizationID, in.TargetUserID, true)
}

// UnblockMemberInput implements `PUT /orgs/{org}/members/{userID}/unblock`.
type UnblockMemberInput struct {
	OrganizationID   string
	RequestingUserID string
	TargetUserID     string
}

type UnblockMemberService struct {
	membershipRepo MembershipRepository
	permChecker    PermissionChecker
}

func NewUnblockMemberService(membershipRepo MembershipRepository, permChecker PermissionChecker) *UnblockMemberService {
	return &UnblockMemberService{membershipRepo: membershipRepo, permChecker: permChecker}
}

func (s *UnblockMemberService) Execute(ctx context.Context, in UnblockMemberInput) error {
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

	// MembershipExists, not IsMember - a blocked target would otherwise
	// fail their own "are you really a member" check, making them
	// permanently unblockable through this exact endpoint.
	targetExists, err := s.membershipRepo.MembershipExists(ctx, in.OrganizationID, in.TargetUserID)
	if err != nil {
		return err
	}
	if !targetExists {
		return domain.ErrOrganizationNotFound
	}

	return s.membershipRepo.SetBlocked(ctx, in.OrganizationID, in.TargetUserID, false)
}

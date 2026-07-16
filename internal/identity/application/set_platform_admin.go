package application

import (
	"context"

	"platform-of-platform/internal/identity/domain"
)

// SetPlatformAdminInput implements `PUT /api/v1/users/{id}/platform-
// admin` - the promote/demote endpoint that lets an existing platform
// admin grant or revoke the same status on another user (Organization
// creation is gated on this flag - see tenancy/application/
// create_organization.go). Self-referential gate: the requester must
// already be a platform admin themselves, checked before the target's
// own flag is touched.
type SetPlatformAdminInput struct {
	RequestingUserID string
	TargetUserID     string
	IsPlatformAdmin  bool
}

type SetPlatformAdminService struct {
	userRepo UserRepository
}

func NewSetPlatformAdminService(userRepo UserRepository) *SetPlatformAdminService {
	return &SetPlatformAdminService{userRepo: userRepo}
}

func (s *SetPlatformAdminService) Execute(ctx context.Context, in SetPlatformAdminInput) error {
	isRequesterAdmin, err := s.userRepo.IsPlatformAdmin(ctx, in.RequestingUserID)
	if err != nil {
		return err
	}
	if !isRequesterAdmin {
		return domain.ErrForbidden
	}

	// A real target - GetByID surfaces domain.ErrUserNotFound for an
	// unknown id rather than silently no-op'ing an UPDATE that matches
	// zero rows.
	if _, err := s.userRepo.GetByID(ctx, in.TargetUserID); err != nil {
		return err
	}

	return s.userRepo.SetPlatformAdmin(ctx, in.TargetUserID, in.IsPlatformAdmin)
}

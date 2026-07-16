package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/identity/application"
	"platform-of-platform/internal/identity/domain"
)

func TestSetPlatformAdminService_NonAdminRequesterGetsForbidden(t *testing.T) {
	repo := newFakeUserRepo()
	requester := mustLocalUser(t, "requester", "hunter2")
	target := mustLocalUser(t, "target", "hunter2")
	repo.put(requester)
	repo.put(target)
	svc := application.NewSetPlatformAdminService(repo)

	err := svc.Execute(context.Background(), application.SetPlatformAdminInput{
		RequestingUserID: requester.ID, TargetUserID: target.ID, IsPlatformAdmin: true,
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-platform-admin requester, got: %v", err)
	}
}

func TestSetPlatformAdminService_ExistingAdminCanPromoteAnotherUser(t *testing.T) {
	repo := newFakeUserRepo()
	requester := mustLocalUser(t, "requester", "hunter2")
	requester.IsPlatformAdmin = true
	target := mustLocalUser(t, "target", "hunter2")
	repo.put(requester)
	repo.put(target)
	svc := application.NewSetPlatformAdminService(repo)

	if err := svc.Execute(context.Background(), application.SetPlatformAdminInput{
		RequestingUserID: requester.ID, TargetUserID: target.ID, IsPlatformAdmin: true,
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	isAdmin, err := repo.IsPlatformAdmin(context.Background(), target.ID)
	if err != nil {
		t.Fatalf("IsPlatformAdmin: %v", err)
	}
	if !isAdmin {
		t.Error("expected the target to be promoted to platform admin")
	}
}

func TestSetPlatformAdminService_UnknownTargetGetsNotFound(t *testing.T) {
	repo := newFakeUserRepo()
	requester := mustLocalUser(t, "requester", "hunter2")
	requester.IsPlatformAdmin = true
	repo.put(requester)
	svc := application.NewSetPlatformAdminService(repo)

	err := svc.Execute(context.Background(), application.SetPlatformAdminInput{
		RequestingUserID: requester.ID, TargetUserID: "nonexistent", IsPlatformAdmin: true,
	})
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got: %v", err)
	}
}

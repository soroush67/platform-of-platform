package application_test

import (
	"context"
	"errors"
	"testing"

	"platform-of-platform/internal/identity/application"
	"platform-of-platform/internal/identity/domain"
)

func TestGetOwnUserService_ReturnsTheCallersOwnUser(t *testing.T) {
	repo := newFakeUserRepo()
	repo.put(mustLocalUser(t, "alice", "hunter2"))
	svc := application.NewGetOwnUserService(repo)

	users, _ := repo.GetByUsername(context.Background(), "alice")
	got, err := svc.Execute(context.Background(), users.ID)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.Username != "alice" {
		t.Errorf("expected alice, got %q", got.Username)
	}
}

func TestGetOwnUserService_UnknownUserIDPropagatesNotFound(t *testing.T) {
	svc := application.NewGetOwnUserService(newFakeUserRepo())

	_, err := svc.Execute(context.Background(), "nonexistent-id")
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got: %v", err)
	}
}

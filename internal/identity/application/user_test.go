package application_test

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"platform-of-platform/internal/identity/application"
	"platform-of-platform/internal/identity/domain"
)

func TestCreateUserService_LocalUserRequiresPassword(t *testing.T) {
	svc := application.NewCreateUserService(newFakeUserRepo())

	_, err := svc.Execute(context.Background(), application.CreateUserInput{
		Username: "alice", Email: "alice@example.com", AuthSource: domain.AuthSourceLocal,
	})
	var validationErr *domain.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError for a local user with no password, got: %v", err)
	}
}

func TestCreateUserService_LocalUserGetsARealBcryptHashNotPlaintext(t *testing.T) {
	repo := newFakeUserRepo()
	svc := application.NewCreateUserService(repo)

	user, err := svc.Execute(context.Background(), application.CreateUserInput{
		Username: "alice", Email: "alice@example.com", AuthSource: domain.AuthSourceLocal, Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if user.PasswordHash == nil {
		t.Fatal("expected PasswordHash to be set")
	}
	if *user.PasswordHash == "correct horse battery staple" {
		t.Fatal("expected a bcrypt hash, not the plaintext password stored verbatim")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte("correct horse battery staple")); err != nil {
		t.Errorf("expected the stored hash to verify against the real password: %v", err)
	}
}

func TestCreateUserService_SSOUserNeedsNoPassword(t *testing.T) {
	repo := newFakeUserRepo()
	svc := application.NewCreateUserService(repo)

	user, err := svc.Execute(context.Background(), application.CreateUserInput{
		Username: "bob", Email: "bob@example.com", AuthSource: domain.AuthSourceOIDC,
	})
	if err != nil {
		t.Fatalf("expected an SSO user to be created without a password, got: %v", err)
	}
	if user.PasswordHash != nil {
		t.Error("expected no password hash for an SSO user")
	}
}

func mustLocalUser(t *testing.T, username, password string) *domain.User {
	t.Helper()
	user, err := domain.NewUser(username, username+"@example.com", domain.AuthSourceLocal)
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword: %v", err)
	}
	if err := user.SetPasswordHash(string(hash)); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}
	return user
}

func TestAuthenticateService_CorrectPasswordSucceeds(t *testing.T) {
	repo := newFakeUserRepo()
	repo.put(mustLocalUser(t, "alice", "hunter2"))
	svc := application.NewAuthenticateService(repo)

	user, err := svc.Execute(context.Background(), "alice", "hunter2")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if user.Username != "alice" {
		t.Errorf("expected alice, got %q", user.Username)
	}
}

// Three genuinely different failure causes - unknown username, wrong
// password, and a non-local (SSO) user attempting password login - must
// all surface as the exact same domain.ErrInvalidCredentials, the
// documented "never let a login form enumerate which usernames exist"
// invariant.
func TestAuthenticateService_AllFailureModesAreIndistinguishable(t *testing.T) {
	repo := newFakeUserRepo()
	repo.put(mustLocalUser(t, "alice", "hunter2"))
	ssoUser, _ := domain.NewUser("carol", "carol@example.com", domain.AuthSourceOIDC)
	repo.put(ssoUser)
	svc := application.NewAuthenticateService(repo)

	cases := []struct {
		name     string
		username string
		password string
	}{
		{"unknown username", "nobody", "whatever"},
		{"wrong password", "alice", "wrong-password"},
		{"sso user attempting password login", "carol", "anything"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Execute(context.Background(), tc.username, tc.password)
			if !errors.Is(err, domain.ErrInvalidCredentials) {
				t.Errorf("expected ErrInvalidCredentials, got: %v", err)
			}
		})
	}
}

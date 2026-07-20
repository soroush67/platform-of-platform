package application_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"platform-of-platform/internal/fleet/application"
	"platform-of-platform/internal/fleet/domain"
)

// ---- minimal in-memory fakes, scoped to exactly what AttachmentService needs ----

type fakeAttachmentMembershipChecker struct {
	members map[string]bool // "orgID|userID"
}

func newFakeAttachmentMembershipChecker() *fakeAttachmentMembershipChecker {
	return &fakeAttachmentMembershipChecker{members: map[string]bool{}}
}
func (f *fakeAttachmentMembershipChecker) add(orgID, userID string) {
	f.members[orgID+"|"+userID] = true
}
func (f *fakeAttachmentMembershipChecker) IsMember(ctx context.Context, organizationID, userID string) (bool, error) {
	return f.members[organizationID+"|"+userID], nil
}

type fakeAttachmentPermChecker struct {
	granted map[string]bool // "orgID|userID|permission"
}

func newFakeAttachmentPermChecker() *fakeAttachmentPermChecker {
	return &fakeAttachmentPermChecker{granted: map[string]bool{}}
}
func (f *fakeAttachmentPermChecker) grant(orgID, userID, permission string) {
	f.granted[orgID+"|"+userID+"|"+permission] = true
}
func (f *fakeAttachmentPermChecker) HasPermission(ctx context.Context, organizationID, userID, permission string) (bool, error) {
	return f.granted[organizationID+"|"+userID+"|"+permission], nil
}

type fakeAttachmentProjectChecker struct {
	projects map[string]bool // "orgID|projectID"
}

func newFakeAttachmentProjectChecker() *fakeAttachmentProjectChecker {
	return &fakeAttachmentProjectChecker{projects: map[string]bool{}}
}
func (f *fakeAttachmentProjectChecker) add(orgID, projectID string) {
	f.projects[orgID+"|"+projectID] = true
}
func (f *fakeAttachmentProjectChecker) ProjectExists(ctx context.Context, organizationID, projectID string) (bool, error) {
	return f.projects[organizationID+"|"+projectID], nil
}

// fakeProjectLinkRepo is a full AttachmentRepository - Network/Volume
// methods are unused no-ops (these tests only exercise the Project
// linking half), Project methods are real, stateful, in-memory.
type fakeProjectLinkRepo struct {
	mu    sync.Mutex
	links map[string]map[string]bool // composeFileID -> set of projectID
}

func newFakeProjectLinkRepo() *fakeProjectLinkRepo {
	return &fakeProjectLinkRepo{links: map[string]map[string]bool{}}
}
func (f *fakeProjectLinkRepo) AttachNetwork(ctx context.Context, actorUserID, organizationID, composeFileID, networkID string) error {
	return nil
}
func (f *fakeProjectLinkRepo) DetachNetwork(ctx context.Context, actorUserID, organizationID, composeFileID, networkID string) error {
	return nil
}
func (f *fakeProjectLinkRepo) ListNetworksForComposeFile(ctx context.Context, organizationID, composeFileID string) ([]*domain.Network, error) {
	return nil, nil
}
func (f *fakeProjectLinkRepo) AttachVolume(ctx context.Context, actorUserID, organizationID, composeFileID, volumeID, containerPath string) error {
	return nil
}
func (f *fakeProjectLinkRepo) DetachVolume(ctx context.Context, actorUserID, organizationID, composeFileID, volumeID string) error {
	return nil
}
func (f *fakeProjectLinkRepo) ListVolumesForComposeFile(ctx context.Context, organizationID, composeFileID string) ([]application.VolumeAttachmentView, error) {
	return nil, nil
}
func (f *fakeProjectLinkRepo) AttachProject(ctx context.Context, actorUserID, organizationID, composeFileID, projectID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.links[composeFileID] == nil {
		f.links[composeFileID] = map[string]bool{}
	}
	f.links[composeFileID][projectID] = true
	return nil
}
func (f *fakeProjectLinkRepo) DetachProject(ctx context.Context, actorUserID, organizationID, composeFileID, projectID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.links[composeFileID], projectID)
	return nil
}
func (f *fakeProjectLinkRepo) ListProjectsForComposeFile(ctx context.Context, organizationID, composeFileID string) ([]application.ProjectSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []application.ProjectSummary
	for projectID := range f.links[composeFileID] {
		out = append(out, application.ProjectSummary{ID: projectID})
	}
	return out, nil
}
func (f *fakeProjectLinkRepo) ListComposeFilesForProject(ctx context.Context, organizationID, projectID string) ([]*domain.ComposeFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*domain.ComposeFile
	for composeFileID, projects := range f.links {
		if projects[projectID] {
			out = append(out, &domain.ComposeFile{ID: composeFileID})
		}
	}
	return out, nil
}

func TestAttachmentService_AttachProject_RequiresComposeFileManage(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakeAttachmentPermChecker()
	projectChecker := newFakeAttachmentProjectChecker()
	projectChecker.add("org-1", "project-1")
	repo := newFakeProjectLinkRepo()
	svc := application.NewAttachmentService(repo, membership, permChecker, projectChecker)

	err := svc.AttachProject(context.Background(), "org-1", "user-1", "cf-1", "project-1")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without compose_file:manage, got: %v", err)
	}

	permChecker.grant("org-1", "user-1", "compose_file:manage")
	if err := svc.AttachProject(context.Background(), "org-1", "user-1", "cf-1", "project-1"); err != nil {
		t.Fatalf("expected attach to succeed once granted, got: %v", err)
	}

	permChecker.grant("org-1", "user-1", "compose_file:read")
	projects, err := svc.ListProjects(context.Background(), "org-1", "user-1", "cf-1")
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 1 || projects[0].ID != "project-1" {
		t.Errorf("expected exactly project-1 linked, got %+v", projects)
	}
}

func TestAttachmentService_AttachProject_RejectsProjectNotInOrg(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakeAttachmentPermChecker()
	permChecker.grant("org-1", "user-1", "compose_file:manage")
	projectChecker := newFakeAttachmentProjectChecker() // deliberately empty - project-1 doesn't exist in org-1
	repo := newFakeProjectLinkRepo()
	svc := application.NewAttachmentService(repo, membership, permChecker, projectChecker)

	err := svc.AttachProject(context.Background(), "org-1", "user-1", "cf-1", "project-1")
	if !errors.Is(err, domain.ErrProjectNotFound) {
		t.Fatalf("expected ErrProjectNotFound for a project id that doesn't belong to this org, got: %v", err)
	}
}

func TestAttachmentService_DetachProject_RequiresComposeFileManage(t *testing.T) {
	membership := newFakeAttachmentMembershipChecker()
	membership.add("org-1", "user-1")
	permChecker := newFakeAttachmentPermChecker()
	permChecker.grant("org-1", "user-1", "compose_file:manage")
	projectChecker := newFakeAttachmentProjectChecker()
	projectChecker.add("org-1", "project-1")
	repo := newFakeProjectLinkRepo()
	svc := application.NewAttachmentService(repo, membership, permChecker, projectChecker)

	if err := svc.AttachProject(context.Background(), "org-1", "user-1", "cf-1", "project-1"); err != nil {
		t.Fatalf("attach setup: %v", err)
	}

	// Revoke and confirm detach is gated the same way attach is.
	noPermChecker := newFakeAttachmentPermChecker()
	svc = application.NewAttachmentService(repo, membership, noPermChecker, projectChecker)
	err := svc.DetachProject(context.Background(), "org-1", "user-1", "cf-1", "project-1")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden without compose_file:manage, got: %v", err)
	}

	noPermChecker.grant("org-1", "user-1", "compose_file:manage")
	if err := svc.DetachProject(context.Background(), "org-1", "user-1", "cf-1", "project-1"); err != nil {
		t.Fatalf("expected detach to succeed once granted, got: %v", err)
	}
	noPermChecker.grant("org-1", "user-1", "compose_file:read")
	projects, err := svc.ListProjects(context.Background(), "org-1", "user-1", "cf-1")
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected no projects linked after detach, got %+v", projects)
	}
}

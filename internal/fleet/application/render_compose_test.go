package application_test

import (
	"strings"
	"testing"

	"platform-of-platform/internal/fleet/application"
	"platform-of-platform/internal/fleet/domain"
)

func TestSubstituteTemplateVars_Succeeds(t *testing.T) {
	out, err := application.SubstituteTemplateVars("image: {{ IMAGE_TAG }}", map[string]string{"IMAGE_TAG": "alpine:3.20"})
	if err != nil {
		t.Fatalf("SubstituteTemplateVars: %v", err)
	}
	if out != "image: alpine:3.20" {
		t.Errorf("expected substituted output, got %q", out)
	}
}

func TestSubstituteTemplateVars_UndefinedKeyFails(t *testing.T) {
	_, err := application.SubstituteTemplateVars("image: {{ MISSING }}", map[string]string{})
	var validationErr *domain.ValidationError
	if err == nil {
		t.Fatal("expected an error for an undefined template variable")
	}
	if !isValidationError(err, &validationErr) {
		t.Fatalf("expected a ValidationError, got: %v", err)
	}
}

func TestRenderCompose_InjectsEnvIntoListEnvironment(t *testing.T) {
	raw := "services:\n  web:\n    image: nginx\n    environment:\n      - EXISTING=keep\n"
	out, err := application.RenderCompose(raw, nil, map[string]string{"NEW_VAR": "new-value", "EXISTING": "should-not-override"}, nil, nil)
	if err != nil {
		t.Fatalf("RenderCompose: %v", err)
	}
	if !strings.Contains(out, "NEW_VAR=new-value") {
		t.Errorf("expected NEW_VAR injected, got:\n%s", out)
	}
	if !strings.Contains(out, "EXISTING=keep") || strings.Contains(out, "should-not-override") {
		t.Errorf("expected the existing EXISTING=keep entry to win, got:\n%s", out)
	}
}

func TestRenderCompose_InjectsEnvIntoMapEnvironment(t *testing.T) {
	raw := "services:\n  web:\n    image: nginx\n    environment:\n      EXISTING: keep\n"
	out, err := application.RenderCompose(raw, nil, map[string]string{"NEW_VAR": "new-value", "EXISTING": "should-not-override"}, nil, nil)
	if err != nil {
		t.Fatalf("RenderCompose: %v", err)
	}
	if !strings.Contains(out, "NEW_VAR: new-value") {
		t.Errorf("expected NEW_VAR injected, got:\n%s", out)
	}
	if !strings.Contains(out, "EXISTING: keep") {
		t.Errorf("expected the existing EXISTING: keep entry to win, got:\n%s", out)
	}
}

func TestRenderCompose_InjectsNetworksAndVolumes(t *testing.T) {
	raw := "services:\n  web:\n    image: nginx\n"
	networks := []*domain.Network{{Name: "shared-net", External: true}}
	volumes := []application.VolumeAttachmentView{{
		Volume:        &domain.Volume{Name: "data", HostPath: "/srv/data"},
		ContainerPath: "/data",
	}}
	out, err := application.RenderCompose(raw, nil, nil, networks, volumes)
	if err != nil {
		t.Fatalf("RenderCompose: %v", err)
	}
	if !strings.Contains(out, "external: true") || !strings.Contains(out, "shared-net") {
		t.Errorf("expected the external network to be injected top-level and per-service, got:\n%s", out)
	}
	if !strings.Contains(out, "/srv/data:/data") {
		t.Errorf("expected the bind-mount string to be injected, got:\n%s", out)
	}
}

func TestRenderCompose_NormalizesLegacyTopLevelServices(t *testing.T) {
	raw := "web:\n  image: nginx\n"
	out, err := application.RenderCompose(raw, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("RenderCompose: %v", err)
	}
	if !strings.Contains(out, "services:") {
		t.Errorf("expected a legacy top-level service key to be normalized under services:, got:\n%s", out)
	}
}

func TestValidateComposeContent_AcceptsRealService(t *testing.T) {
	if err := application.ValidateComposeContent("services:\n  web:\n    image: nginx\n"); err != nil {
		t.Errorf("expected valid compose content to pass, got: %v", err)
	}
}

func TestValidateComposeContent_RejectsServiceWithNoImageOrBuild(t *testing.T) {
	err := application.ValidateComposeContent("services:\n  web:\n    ports:\n      - \"80:80\"\n")
	if err == nil {
		t.Fatal("expected a service with neither image nor build to fail validation")
	}
}

func TestValidateComposeContent_RejectsEmptyServices(t *testing.T) {
	if err := application.ValidateComposeContent("services: {}\n"); err == nil {
		t.Error("expected empty services: to fail validation")
	}
}

func TestValidateComposeContent_RejectsInvalidYAML(t *testing.T) {
	if err := application.ValidateComposeContent("not: valid: yaml: at: all:"); err == nil {
		t.Error("expected malformed YAML to fail validation")
	}
}

func isValidationError(err error, target **domain.ValidationError) bool {
	if ve, ok := err.(*domain.ValidationError); ok {
		*target = ve
		return true
	}
	return false
}

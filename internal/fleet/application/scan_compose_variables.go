package application

import (
	"context"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"platform-of-platform/internal/fleet/domain"
)

// scanComposeEnvironment extracts every service's own `environment:`
// block (docker-compose's own list-of-"KEY=value" or map-of-KEY:value
// shape) into one flat key->value map - the "scan on save" half of
// syncing a Compose file's inline env vars into real, editable
// fleet_variables (var_type=env). Same YAML parsing shape
// ValidateComposeContent/RenderCompose already use (normalizeTopLevel,
// render_compose.go). Later services win on a key collision within the
// same file - fleet_variables are per-ComposeFile, not per-service, so
// there's no finer scope to preserve the distinction at.
func scanComposeEnvironment(content string) (map[string]string, error) {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return nil, err
	}
	doc = normalizeTopLevel(doc)
	services, _ := doc["services"].(map[string]any)

	found := map[string]string{}
	for _, svc := range services {
		svcMap, ok := svc.(map[string]any)
		if !ok {
			continue
		}
		switch env := svcMap["environment"].(type) {
		case []any:
			// List form: "KEY=value" or a bare "KEY" (pass-through from
			// the host shell, no literal value in the file itself).
			for _, item := range env {
				s, ok := item.(string)
				if !ok {
					continue
				}
				key, value, _ := strings.Cut(s, "=")
				found[key] = value
			}
		case map[string]any:
			// Map form: "KEY: value" - a null/empty value means the same
			// pass-through-from-shell case as a bare list entry.
			for key, v := range env {
				if v == nil {
					found[key] = ""
					continue
				}
				found[key] = fmt.Sprintf("%v", v)
			}
		}
	}
	return found, nil
}

// syncEnvVariablesFromCompose upserts a var_type=env fleet_variable for
// every KEY found in the compose content's own environment: blocks -
// operator's own explicit choice: the YAML is the source of truth, so
// an existing env-typed variable with the same key gets its Value
// replaced on every save. A key already claimed by a DIFFERENT var_type
// (kv/secret/file_template/config_file) is left alone, never silently
// reassigned - fleet_variables.key is unique per ComposeFile regardless
// of var_type (migrations/0019_fleet.up.sql), so blindly overwriting
// could break a deliberately-configured {{ }} substitution binding, and
// there'd be nothing sensible to do with a secret-typed variable's
// SecretRef anyway. Called after a successful Create/UpdateContent, on
// every save (operator's own explicit choice - no separate "Import"
// step) - propagates a real error on failure rather than swallowing it,
// since the compose content itself was already YAML-validated moments
// earlier in the same call.
func syncEnvVariablesFromCompose(ctx context.Context, variables VariableRepository, actorUserID, organizationID, composeFileID, content string) error {
	found, err := scanComposeEnvironment(content)
	if err != nil {
		return err
	}

	existing, err := variables.ListByComposeFile(ctx, organizationID, composeFileID)
	if err != nil {
		return err
	}
	byKey := make(map[string]*domain.Variable, len(existing))
	for _, v := range existing {
		byKey[v.Key] = v
	}

	for key, value := range found {
		if current, ok := byKey[key]; ok {
			if current.VarType != domain.VarTypeEnv {
				continue
			}
			current.Value = value
			if err := variables.Update(ctx, actorUserID, current); err != nil {
				return err
			}
			continue
		}
		v, err := domain.NewVariable(organizationID, composeFileID, key, domain.VarTypeEnv, value, "")
		if err != nil {
			return err
		}
		if err := variables.Create(ctx, actorUserID, v); err != nil {
			return err
		}
	}
	return nil
}

package application

import (
	"context"

	"platform-of-platform/internal/fleet/domain"
)

// ResolvedComposeVariables is DeployExecutor's own view of a
// ComposeFile's real, fully-resolved Variables - split by how
// RenderCompose actually consumes each kind, matching the ported
// Python product's own resolve_variables + build_compose split.
type ResolvedComposeVariables struct {
	// TemplateValues - every kv/secret/file_template/config_file
	// Variable's real value, available as a {{ KEY }} substitution
	// target. Matches the Python original's own "every resolved
	// variable is a template context key, not just kv/secret."
	TemplateValues map[string]string
	// EnvVars - only var_type=env, auto-injected into every service's
	// environment: without needing an explicit {{ }} reference.
	EnvVars map[string]string
	// FileVariables - the file_template/config_file Variables
	// themselves (not yet substituted) - DeployExecutor substitutes
	// each one's own Value against TemplateValues and writes the result
	// as a RemoteFile at FileTargetPath.
	FileVariables []*domain.Variable
}

// resolveComposeVariables implements the same cascade the ported Python
// product's own resolve_variables did: this ComposeFile's own Variables
// win, falling back to the Organization's global ComposeFile's
// Variables (if one exists and isn't this same ComposeFile) for any key
// not locally overridden. Secret-typed values are resolved eagerly here
// (needed both for real template substitution and for DeployExecutor's
// own per-line output scrubbing) via secretResolver - never persisted,
// same live-resolution posture as internal/variables' own
// ResolveVariableService.
func resolveComposeVariables(ctx context.Context, organizationID, composeFileID string, variables VariableRepository, composeFiles ComposeFileRepository, secretResolver SecretResolver) (*ResolvedComposeVariables, error) {
	local, err := variables.ListByComposeFile(ctx, organizationID, composeFileID)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(local))
	for _, v := range local {
		seen[v.Key] = true
	}

	all := local
	if global, found, err := composeFiles.GetGlobal(ctx, organizationID); err != nil {
		return nil, err
	} else if found && global.ID != composeFileID {
		fallback, err := variables.ListByComposeFile(ctx, organizationID, global.ID)
		if err != nil {
			return nil, err
		}
		for _, v := range fallback {
			if !seen[v.Key] {
				all = append(all, v)
			}
		}
	}

	resolved := &ResolvedComposeVariables{
		TemplateValues: map[string]string{},
		EnvVars:        map[string]string{},
	}
	for _, v := range all {
		value := v.Value
		if v.VarType == domain.VarTypeSecret {
			resolvedSecret, err := secretResolver.ResolveValue(ctx, organizationID, v.SecretRef.MountID, v.SecretRef.Path)
			if err != nil {
				return nil, err
			}
			value = resolvedSecret
		}

		switch v.VarType {
		case domain.VarTypeEnv:
			resolved.EnvVars[v.Key] = value
			resolved.TemplateValues[v.Key] = value
		case domain.VarTypeFileTemplate, domain.VarTypeConfigFile:
			resolved.TemplateValues[v.Key] = value
			cp := *v
			cp.Value = value
			resolved.FileVariables = append(resolved.FileVariables, &cp)
		default: // kv, secret
			resolved.TemplateValues[v.Key] = value
		}
	}

	return resolved, nil
}

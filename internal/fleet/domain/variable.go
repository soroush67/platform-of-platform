package domain

import (
	"time"

	"github.com/google/uuid"
)

// VarType is the closed set of 5 kinds a ComposeFile's own Variable can
// be - ported directly from the Python product's own var_type enum.
type VarType string

const (
	// VarTypeKV is a plain named value, substituted only where the
	// compose file's own {{ KEY }} placeholder text explicitly
	// references it.
	VarTypeKV VarType = "kv"
	// VarTypeSecret resolves through SecretRef (never Value) - same
	// {{ KEY }} substitution as kv, but the real value is fetched live
	// via SecretResolver at render/deploy time, never stored plaintext.
	VarTypeSecret VarType = "secret"
	// VarTypeEnv is auto-injected into every service's environment: in
	// the rendered compose file (application/render_compose.go's own
	// injectEnv) - no explicit {{ }} reference needed, unlike kv/secret.
	VarTypeEnv VarType = "env"
	// VarTypeFileTemplate/VarTypeConfigFile both require FileTargetPath -
	// their Value is rendered through the same {{ }} substitution and
	// SFTP-written to the target machine alongside the compose file
	// itself, at FileTargetPath relative to the deploy directory.
	VarTypeFileTemplate VarType = "file_template"
	VarTypeConfigFile   VarType = "config_file"
)

func (t VarType) Valid() bool {
	switch t {
	case VarTypeKV, VarTypeSecret, VarTypeEnv, VarTypeFileTemplate, VarTypeConfigFile:
		return true
	}
	return false
}

func (t VarType) RequiresFileTargetPath() bool {
	return t == VarTypeFileTemplate || t == VarTypeConfigFile
}

// Variable is a ComposeFile's own key/value - Value XOR SecretRef,
// exactly the same pattern variables/domain.Variable already proved out
// (and the same migrations/0019_fleet.up.sql CHECK constraint at the
// storage layer). ComposeFileID is always a real compose_files.id - no
// nullable-FK-means-"the global one" special case the Python schema
// has; a "global" Variable is just one created against the
// Organization's own IsGlobal ComposeFile.
type Variable struct {
	ID             string
	OrganizationID string
	ComposeFileID  string
	Key            string
	VarType        VarType
	Value          string
	SecretRef      *SecretReference
	FileTargetPath string
	CreatedAt      time.Time
}

func validateVariableFields(organizationID, composeFileID, key string, varType VarType, fileTargetPath string) error {
	if organizationID == "" || composeFileID == "" {
		return &ValidationError{Message: "organization_id and compose_file_id are required"}
	}
	if key == "" {
		return &ValidationError{Message: "key is required"}
	}
	if !varType.Valid() {
		return &ValidationError{Message: "invalid var_type: " + string(varType)}
	}
	if varType.RequiresFileTargetPath() && fileTargetPath == "" {
		return &ValidationError{Message: "file_target_path is required for var_type " + string(varType)}
	}
	return nil
}

func NewVariable(organizationID, composeFileID, key string, varType VarType, value, fileTargetPath string) (*Variable, error) {
	if err := validateVariableFields(organizationID, composeFileID, key, varType, fileTargetPath); err != nil {
		return nil, err
	}
	if varType == VarTypeSecret {
		return nil, &ValidationError{Message: "var_type secret must be created via NewVariableWithSecretRef"}
	}

	return &Variable{
		ID:             uuid.NewString(),
		OrganizationID: organizationID,
		ComposeFileID:  composeFileID,
		Key:            key,
		VarType:        varType,
		Value:          value,
		FileTargetPath: fileTargetPath,
		CreatedAt:      time.Now().UTC(),
	}, nil
}

// NewVariableWithSecretRef is NewVariable's counterpart for
// VarTypeSecret - mountID/path aren't validated for real existence here
// (Fleet never imports secrets/domain; CreateVariableService's own
// SecretMountChecker port does that), only for non-emptiness.
func NewVariableWithSecretRef(organizationID, composeFileID, key string, mountID, path string) (*Variable, error) {
	if err := validateVariableFields(organizationID, composeFileID, key, VarTypeSecret, ""); err != nil {
		return nil, err
	}
	if mountID == "" || path == "" {
		return nil, &ValidationError{Message: "secret_mount_id and secret_path are required"}
	}

	return &Variable{
		ID:             uuid.NewString(),
		OrganizationID: organizationID,
		ComposeFileID:  composeFileID,
		Key:            key,
		VarType:        VarTypeSecret,
		SecretRef:      &SecretReference{MountID: mountID, Path: path},
		CreatedAt:      time.Now().UTC(),
	}, nil
}

package engine

// OpenTofuEngine is this codebase's fourth real engine - shares its
// entire Execute implementation with TerraformEngine via the embedded
// terraformFamilyEngine (terraform.go), only the invoked binary differs
// (`tofu`, not `terraform`). Same deliberate scope as TerraformEngine:
// single-shot apply only, local-only providers, no persisted state
// across runs (terraformFamilyEngine's own doc comment has the full
// reasoning - it applies identically here).
//
// This is the Apache-2.0, wire-compatible alternative TerraformEngine's
// own doc comment names - built for real alongside Terraform, not as a
// hypothetical someday-swap.
type OpenTofuEngine struct{ terraformFamilyEngine }

func NewOpenTofuEngine() *OpenTofuEngine {
	return &OpenTofuEngine{terraformFamilyEngine{binary: "tofu"}}
}

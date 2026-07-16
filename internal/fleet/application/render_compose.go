package application

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"platform-of-platform/internal/fleet/domain"
)

// placeholderPattern matches {{ KEY }}-style references inside a
// ComposeFile's raw content (or a file_template/config_file Variable's
// own content) - the same substitution convention the ported Python
// product's own Jinja2-based render_template used, reimplemented as a
// direct regex replace rather than fighting Go's text/template into
// accepting bare {{ KEY }} syntax (its own dotted-field access rules
// aren't compatible with it without rewriting every ported compose
// file's placeholders).
var placeholderPattern = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

// SubstituteTemplateVars validates every {{ KEY }} reference resolves
// before replacing any of them - the same "fail the whole render if any
// referenced key is unresolved" guard Jinja2's StrictUndefined gave the
// Python original, not a partial/best-effort substitution.
func SubstituteTemplateVars(content string, values map[string]string) (string, error) {
	var missing []string
	seen := map[string]bool{}
	for _, match := range placeholderPattern.FindAllStringSubmatch(content, -1) {
		key := match[1]
		if seen[key] {
			continue
		}
		seen[key] = true
		if _, ok := values[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return "", &domain.ValidationError{Message: "undefined template variable(s): " + strings.Join(missing, ", ")}
	}

	return placeholderPattern.ReplaceAllStringFunc(content, func(m string) string {
		key := placeholderPattern.FindStringSubmatch(m)[1]
		return values[key]
	}), nil
}

// MaskedValue is what a secret-typed value gets replaced with for the
// read-only rendered-preview path (GetComposeFileService's own
// "rendered" view) - the caller (resolve_compose_variables.go) swaps
// secret values for this BEFORE calling RenderCompose, matching the
// ported Python original's own build_compose(mask_secrets=True)
// ordering (mask before Jinja substitution, not after) - RenderCompose
// itself has no knowledge of which values were ever secret-typed.
const MaskedValue = "••••••••"

// RenderCompose is the structural half of rendering - {{ }} template
// substitution (values, already resolved and optionally pre-masked by
// the caller) happens first, then a structural YAML merge: parses into
// map[string]any (not yaml.Node - comment/key-order preservation isn't
// needed here, PyYAML's own safe_load/safe_dump already discarded
// both), walks the same normalize -> inject-env -> inject-networks ->
// inject-volumes steps compose_builder.py's own build_compose does,
// re-dumps.
func RenderCompose(rawContent string, values map[string]string, envVars map[string]string, networks []*domain.Network, volumes []VolumeAttachmentView) (string, error) {
	substituted, err := SubstituteTemplateVars(rawContent, values)
	if err != nil {
		return "", err
	}

	var doc map[string]any
	if err := yaml.Unmarshal([]byte(substituted), &doc); err != nil {
		return "", &domain.ValidationError{Message: "compose content is not valid YAML: " + err.Error()}
	}
	if doc == nil {
		doc = map[string]any{}
	}

	doc = normalizeTopLevel(doc)

	services, _ := doc["services"].(map[string]any)
	if services == nil {
		services = map[string]any{}
		doc["services"] = services
	}

	if len(envVars) > 0 {
		for _, svc := range services {
			if svcMap, ok := svc.(map[string]any); ok {
				injectEnv(svcMap, envVars)
			}
		}
	}

	if len(networks) > 0 {
		injectNetworks(doc, services, networks)
	}
	if len(volumes) > 0 {
		injectVolumes(services, volumes)
	}

	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// reservedTopLevelKeys - normalizeTopLevel treats every OTHER top-level
// key as a legacy (Compose v1-style, no explicit `services:`) service
// definition, matching the Python original's own normalize_top_level
// heuristic exactly.
var reservedTopLevelKeys = map[string]bool{
	"version": true, "networks": true, "volumes": true, "configs": true, "secrets": true, "services": true,
}

func normalizeTopLevel(doc map[string]any) map[string]any {
	if _, hasServices := doc["services"]; hasServices {
		return doc
	}
	services := map[string]any{}
	normalized := map[string]any{}
	for k, v := range doc {
		if reservedTopLevelKeys[k] {
			normalized[k] = v
			continue
		}
		services[k] = v
	}
	normalized["services"] = services
	return normalized
}

// injectEnv - list-vs-map environment: handling, existing entries
// always win (never overwritten), matching _merge_env exactly.
func injectEnv(service map[string]any, envVars map[string]string) {
	switch env := service["environment"].(type) {
	case []any:
		existing := map[string]bool{}
		for _, e := range env {
			if s, ok := e.(string); ok {
				if idx := strings.IndexByte(s, '='); idx >= 0 {
					existing[s[:idx]] = true
				} else {
					existing[s] = true
				}
			}
		}
		keys := sortedKeys(envVars)
		for _, k := range keys {
			if !existing[k] {
				env = append(env, fmt.Sprintf("%s=%s", k, envVars[k]))
			}
		}
		service["environment"] = env
	case map[string]any:
		for _, k := range sortedKeys(envVars) {
			if _, ok := env[k]; !ok {
				env[k] = envVars[k]
			}
		}
	case nil:
		m := map[string]any{}
		for _, k := range sortedKeys(envVars) {
			m[k] = envVars[k]
		}
		service["environment"] = m
	}
}

// injectNetworks - top-level networks: entry per attached Network
// (external flag), plus a per-service reference appended to every
// service's own networks: list if not already present. Compose-file-
// wide, not per-service - same deliberate simplification the Python
// original's own comment already names (most ComposeFiles here are
// single-service).
func injectNetworks(doc map[string]any, services map[string]any, networks []*domain.Network) {
	topLevel, _ := doc["networks"].(map[string]any)
	if topLevel == nil {
		topLevel = map[string]any{}
	}
	for _, n := range networks {
		topLevel[n.Name] = map[string]any{"external": n.External}
	}
	doc["networks"] = topLevel

	for _, svc := range services {
		svcMap, ok := svc.(map[string]any)
		if !ok {
			continue
		}
		existing := stringSetFromList(svcMap["networks"])
		list, _ := svcMap["networks"].([]any)
		for _, n := range networks {
			if !existing[n.Name] {
				list = append(list, n.Name)
				existing[n.Name] = true
			}
		}
		svcMap["networks"] = list
	}
}

// injectVolumes - per-service bind-mount strings ("host_path:
// container_path"), deduped against existing entries, compose-file-wide
// same as injectNetworks.
func injectVolumes(services map[string]any, volumes []VolumeAttachmentView) {
	for _, svc := range services {
		svcMap, ok := svc.(map[string]any)
		if !ok {
			continue
		}
		existing := stringSetFromList(svcMap["volumes"])
		list, _ := svcMap["volumes"].([]any)
		for _, va := range volumes {
			bind := fmt.Sprintf("%s:%s", va.Volume.HostPath, va.ContainerPath)
			if !existing[bind] {
				list = append(list, bind)
				existing[bind] = true
			}
		}
		svcMap["volumes"] = list
	}
}

func stringSetFromList(v any) map[string]bool {
	set := map[string]bool{}
	if list, ok := v.([]any); ok {
		for _, item := range list {
			if s, ok := item.(string); ok {
				set[s] = true
			}
		}
	}
	return set
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ValidateComposeContent is a direct port of the Python original's own
// compose_validator.py structural check - valid YAML, normalizes to a
// non-empty services: mapping (via the same normalizeTopLevel
// heuristic), every service has image or build. Deliberately does NOT
// run `docker compose config` and does NOT substitute {{ }} - a compose
// file whose only real content is behind unresolved placeholders is
// still structurally valid, exactly matching the Python original's own
// documented scope.
func ValidateComposeContent(content string) error {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return &domain.ValidationError{Message: "compose content is not valid YAML: " + err.Error()}
	}
	if doc == nil {
		return &domain.ValidationError{Message: "compose content must not be empty"}
	}

	doc = normalizeTopLevel(doc)
	services, _ := doc["services"].(map[string]any)
	if len(services) == 0 {
		return &domain.ValidationError{Message: "compose content must define at least one service"}
	}
	for name, svc := range services {
		svcMap, ok := svc.(map[string]any)
		if !ok {
			return &domain.ValidationError{Message: fmt.Sprintf("service %q must be a mapping", name)}
		}
		_, hasImage := svcMap["image"]
		_, hasBuild := svcMap["build"]
		if !hasImage && !hasBuild {
			return &domain.ValidationError{Message: fmt.Sprintf("service %q must have an image or build", name)}
		}
	}
	return nil
}

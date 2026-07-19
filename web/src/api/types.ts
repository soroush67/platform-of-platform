// Flat TypeScript mirror of the Go control-plane's REST contract
// (cmd/control-plane/main.go's route table + each adapters/http
// package's request/response DTOs) - field names match the JSON tags
// exactly (snake_case), not renamed for JS convention, so a response
// body can be cast straight into these types with zero mapping code.

export interface ProblemDetails {
  type: string;
  title: string;
  status: number;
  detail?: string;
  request_id?: string;
}

export class ApiError extends Error {
  status: number;
  detail?: string;
  constructor(problem: ProblemDetails) {
    super(problem.title);
    this.status = problem.status;
    this.detail = problem.detail;
  }
}

// ---- Identity ----

export interface User {
  id: string;
  username: string;
  email: string;
  auth_source: string;
  status: string;
  created_at: string;
  is_platform_admin: boolean;
}

export interface LoginResponse {
  access_token: string;
  token_type: string;
  expires_in: number;
  refresh_token: string;
}

export interface ServiceAccount {
  id: string;
  organization_id: string;
  name: string;
  description: string;
  created_at: string;
}

export interface ApiKeyCreateResponse {
  id: string;
  owner_type: string;
  owner_id: string;
  name: string;
  scopes: string[];
  expires_at: string;
  last_used_at?: string;
  created_at: string;
  key: string; // only ever present on the create response - shown once
}

// ---- Tenancy ----

export interface Organization {
  id: string;
  name: string;
  slug: string;
  status: string;
  created_at: string;
}

export interface Team {
  id: string;
  organization_id: string;
  name: string;
  created_at: string;
}

export interface Project {
  id: string;
  organization_id: string;
  name: string;
  slug: string;
  description: string;
  created_at: string;
}

// ROLE_NAMES is the closed set ChangeMemberRoleService actually accepts
// (internal/tenancy/application/change_member_role.go's own
// validRoleNames) - the 4 built-in roles only, not any custom role a
// RolesPage-created Role might have. A member's org-scope "role" (as
// opposed to a fine-grained RoleBinding at project/workspace scope,
// which can be any role) is always one of these.
export const ROLE_NAMES = ["owner", "admin", "write", "read"] as const;
export type RoleName = (typeof ROLE_NAMES)[number];

export interface Member {
  user_id: string;
  username: string;
  email: string;
  role_name: string;
  joined_at: string;
  // blocked - per-organization suspension only (BlockMemberService) -
  // the member stays a real platform user and keeps working in any
  // other org they belong to.
  blocked: boolean;
}

// AvailableUser is a platform User not yet a member of the org the
// Members page is currently showing - GET /orgs/{id}/members/available's
// own response shape, backs the "add existing user" picker.
export interface AvailableUser {
  id: string;
  username: string;
  email: string;
}

// TeamMember is Member's own sibling for a Team's roster - no role_name
// (Team membership itself carries no per-team role, see
// TeamMemberSummary's own comment server-side).
export interface TeamMember {
  user_id: string;
  username: string;
  email: string;
  joined_at: string;
}

// ---- RBAC ----

// Mirrors internal/rbac/domain/role.go's AllPermissions - each Fleet
// menu (Machines / Networks & volumes / Compose files / Operations) has
// its own independent permission pair, same for Projects, so a custom
// Role can grant e.g. "manage Machines" without also granting "manage
// Operations."
export const PERMISSIONS = [
  "organization:read",
  "organization:manage",
  "organization:delete",
  "project:read",
  "project:manage",
  "workspace:read",
  "workspace:manage",
  "workspace:apply",
  "machine:read",
  "machine:manage",
  "network_volume:read",
  "network_volume:manage",
  "compose_file:read",
  "compose_file:manage",
  "operation:read",
  "operation:deploy",
] as const;
export type Permission = (typeof PERMISSIONS)[number];

// PERMISSION_GROUP_LABELS/PERMISSION_GROUPS - RolesPage's grouped
// checkbox UI needs permissions bucketed by resource (the part before
// ":"), matching the exact same one-permission-pair-per-menu split
// AllPermissions itself already documents (server-side comment above) -
// derived from PERMISSIONS itself, not a second hand-maintained list.
const PERMISSION_GROUP_LABELS: Record<string, string> = {
  organization: "Organization",
  project: "Projects",
  workspace: "Workspaces",
  machine: "Machines",
  network_volume: "Networks & volumes",
  compose_file: "Compose files",
  operation: "Operations",
};

export interface PermissionGroup {
  key: string;
  label: string;
  permissions: Permission[];
}

export const PERMISSION_GROUPS: PermissionGroup[] = (() => {
  const order: string[] = [];
  const byKey = new Map<string, Permission[]>();
  for (const p of PERMISSIONS) {
    const key = p.split(":")[0];
    if (!byKey.has(key)) {
      byKey.set(key, []);
      order.push(key);
    }
    byKey.get(key)!.push(p);
  }
  return order.map((key) => ({ key, label: PERMISSION_GROUP_LABELS[key] ?? key, permissions: byKey.get(key)! }));
})();

export interface Role {
  id: string;
  organization_id: string | null;
  name: string;
  permissions: string[];
}

// RoleBinding - *_name fields are resolved server-side
// (ListRoleBindingsService, list_role_bindings.go) - "" when
// unresolvable (a since-deleted resource) or when scope_type is
// "organization" (no lookup needed for that one case, see
// RoleBindingsPage's own handling).
export interface RoleBinding {
  id: string;
  organization_id: string;
  role_id: string;
  role_name: string;
  subject_type: string;
  subject_id: string;
  subject_name: string;
  scope_type: string;
  scope_id: string;
  scope_name: string;
  effect: string;
  created_at: string;
}

// ---- Workspace ----

export interface Environment {
  id: string;
  organization_id: string;
  project_id: string;
  name: string;
  promotion_rank: number;
  requires_approval: boolean;
  created_at: string;
}

// EXECUTION_ENGINES mirrors internal/workspace/domain's closed
// ExecutionEngine enum (8 values) - all 8 have a real Worker-side
// implementation now (internal/worker/engine).
export const EXECUTION_ENGINES = [
  "compose",
  "terraform",
  "opentofu",
  "ansible",
  "helm",
  "packer",
  "kubespray",
  "kubernetes",
] as const;

export interface Workspace {
  id: string;
  organization_id: string;
  project_id: string;
  environment_id?: string;
  name: string;
  execution_engine: string;
  locked: boolean;
  created_at: string;
}

// ---- Execution ----

export type RunStatus =
  | "queued"
  | "planning"
  | "planned"
  | "policy_check"
  | "awaiting_approval"
  | "applying"
  | "applied"
  | "failed"
  | "errored"
  | "canceled";

export const TERMINAL_RUN_STATUSES: RunStatus[] = ["applied", "failed", "errored", "canceled"];

export function isTerminalRunStatus(status: RunStatus): boolean {
  return TERMINAL_RUN_STATUSES.includes(status);
}

export interface Run {
  id: string;
  organization_id: string;
  workspace_id: string;
  trigger: string;
  triggered_by: string;
  status: RunStatus;
  created_at: string;
  finished_at?: string;
  apply_output_ref?: string;
}

// ---- Variables ----

export interface SecretRef {
  mount_id: string;
  path: string;
}

export interface Variable {
  id: string;
  organization_id: string;
  scope_type: string;
  scope_id: string;
  key: string;
  category: string;
  sensitivity: string;
  value: string | null;
  secret_ref: SecretRef | null;
  created_at: string;
}

export interface CreateVariableRequest {
  scope_type: string;
  scope_id: string;
  key: string;
  category: string;
  sensitivity: string;
  value?: string;
  secret_ref?: SecretRef | null;
}

// ---- Secrets ----

export interface SecretMount {
  id: string;
  organization_id: string;
  name: string;
  backend_type: string;
  address: string;
  role_id: string;
  created_at: string;
}

// ---- Audit ----

export interface AuditLogEntry {
  id: string;
  actor: string;
  action: string;
  target_type: string;
  target_id: string;
  metadata: Record<string, unknown>;
  created_at: string;
}

export interface AuditLogPage {
  data: AuditLogEntry[];
  next_cursor?: string;
}

export interface ListResponse<T> {
  data: T[];
}

// ---- Fleet ----
// Mirrors internal/fleet/adapters/http's response DTOs - the docker-
// compose fleet management context ported from compose-platform (see
// the Fleet plan). "FleetVariable" (not "Variable" - already taken by
// the Variables context's own workspace-scoped Variable above) is a
// per-ComposeFile variable, a different resource entirely.

export const CONNECTION_STATUSES = ["unknown", "online", "unreachable"] as const;
export const DOCKER_STATUSES = ["unknown", "ok", "missing", "error"] as const;
export const CREDENTIAL_TYPES = ["ssh_key", "ssh_password"] as const;
export type CredentialType = (typeof CREDENTIAL_TYPES)[number];

export interface Machine {
  id: string;
  organization_id: string;
  name: string;
  host: string;
  ssh_port: number;
  ssh_user: string;
  credential_type: CredentialType;
  credential_mount_id: string;
  credential_path: string;
  deploy_base_path: string;
  connection_status: (typeof CONNECTION_STATUSES)[number];
  docker_status: (typeof DOCKER_STATUSES)[number];
  last_checked_at?: string;
  archived: boolean;
  created_at: string;
}

export interface FleetNetwork {
  id: string;
  organization_id: string;
  name: string;
  external: boolean;
  created_by: string;
  created_at: string;
}

export interface FleetVolume {
  id: string;
  organization_id: string;
  name: string;
  host_path: string;
  created_by: string;
  created_at: string;
}

export interface ComposeFile {
  id: string;
  organization_id: string;
  name: string;
  is_global: boolean;
  compose_content: string;
  created_by: string;
  created_at: string;
}

export interface VolumeAttachment {
  volume: FleetVolume;
  container_path: string;
}

export const VAR_TYPES = ["kv", "secret", "env", "file_template", "config_file"] as const;
export type VarType = (typeof VAR_TYPES)[number];

export interface FleetVariable {
  id: string;
  organization_id: string;
  compose_file_id: string;
  key: string;
  var_type: VarType;
  value?: string | null;
  secret_ref?: SecretRef | null;
  file_target_path?: string;
  created_at: string;
}

export const OPERATION_TYPES = ["deploy", "up", "down", "restart", "pull", "build", "stop", "start", "remove"] as const;
export type OperationType = (typeof OPERATION_TYPES)[number];

export type OperationStatus = "queued" | "running" | "success" | "failed";

export const TERMINAL_OPERATION_STATUSES: OperationStatus[] = ["success", "failed"];

export function isTerminalOperationStatus(status: OperationStatus): boolean {
  return TERMINAL_OPERATION_STATUSES.includes(status);
}

export interface Operation {
  id: string;
  organization_id: string;
  compose_file_id: string;
  machine_id: string;
  operation_type: OperationType;
  status: OperationStatus;
  triggered_by: string;
  created_at: string;
  started_at?: string;
  finished_at?: string;
  exit_code?: number;
  output?: string;
}

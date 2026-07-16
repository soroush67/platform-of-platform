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
}

// ---- RBAC ----

export const PERMISSIONS = [
  "organization:read",
  "organization:manage",
  "organization:delete",
  "workspace:read",
  "workspace:manage",
  "workspace:apply",
] as const;
export type Permission = (typeof PERMISSIONS)[number];

export interface Role {
  id: string;
  organization_id: string | null;
  name: string;
  permissions: string[];
}

export interface RoleBinding {
  id: string;
  organization_id: string;
  role_id: string;
  subject_type: string;
  subject_id: string;
  scope_type: string;
  scope_id: string;
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

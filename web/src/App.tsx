import { Navigate, Route, Routes } from "react-router-dom";

import { RequireAuth } from "./auth/RequireAuth";
import { OrgLayout } from "./layout/OrgLayout";
import { LoginPage } from "./pages/LoginPage";
import { OrgListPage } from "./pages/OrgListPage";
import { PlatformAdminPage } from "./pages/PlatformAdminPage";
import { OrgOverviewPage } from "./pages/OrgOverviewPage";
import { ProjectListPage } from "./pages/ProjectListPage";
import { ProjectDetailPage } from "./pages/ProjectDetailPage";
import { EnvironmentDetailPage } from "./pages/EnvironmentDetailPage";
import { WorkspaceDetailPage } from "./pages/WorkspaceDetailPage";
import { RunDetailPage } from "./pages/RunDetailPage";
import { MachinesPage } from "./pages/MachinesPage";
import { NetworksVolumesPage } from "./pages/NetworksVolumesPage";
import { ComposeFilesPage } from "./pages/ComposeFilesPage";
import { ComposeFileDetailPage } from "./pages/ComposeFileDetailPage";
import { OperationsPage } from "./pages/OperationsPage";
import { OperationDetailPage } from "./pages/OperationDetailPage";
import { VariablesPage } from "./pages/VariablesPage";
import { SecretMountsPage } from "./pages/SecretMountsPage";
import { RolesPage } from "./pages/RolesPage";
import { RoleBindingsPage } from "./pages/RoleBindingsPage";
import { TeamsPage } from "./pages/TeamsPage";
import { MembersPage } from "./pages/MembersPage";
import { ServiceAccountsPage } from "./pages/ServiceAccountsPage";
import { ServiceAccountDetailPage } from "./pages/ServiceAccountDetailPage";
import { AuditLogPage } from "./pages/AuditLogPage";
import { OrgSettingsPage } from "./pages/OrgSettingsPage";
import { NotFoundPage } from "./pages/NotFoundPage";

export function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/" element={<Navigate to="/orgs" replace />} />

      <Route
        path="/orgs"
        element={
          <RequireAuth>
            <OrgListPage />
          </RequireAuth>
        }
      />

      <Route
        path="/orgs/:orgId"
        element={
          <RequireAuth>
            <OrgLayout />
          </RequireAuth>
        }
      >
        <Route index element={<OrgOverviewPage />} />
        <Route path="platform-admin" element={<PlatformAdminPage />} />
        <Route path="projects" element={<ProjectListPage />} />
        <Route path="projects/:projectId" element={<ProjectDetailPage />} />
        <Route path="projects/:projectId/environments/:envId" element={<EnvironmentDetailPage />} />
        <Route path="projects/:projectId/workspaces/:workspaceId" element={<WorkspaceDetailPage />} />
        <Route
          path="projects/:projectId/workspaces/:workspaceId/runs/:runId"
          element={<RunDetailPage />}
        />
        <Route path="machines" element={<MachinesPage />} />
        <Route path="networks-volumes" element={<NetworksVolumesPage />} />
        <Route path="compose-files" element={<ComposeFilesPage />} />
        <Route path="compose-files/:composeFileId" element={<ComposeFileDetailPage />} />
        <Route path="operations" element={<OperationsPage />} />
        <Route path="operations/:operationId" element={<OperationDetailPage />} />
        <Route path="variables" element={<VariablesPage />} />
        <Route path="secret-mounts" element={<SecretMountsPage />} />
        <Route path="roles" element={<RolesPage />} />
        <Route path="role-bindings" element={<RoleBindingsPage />} />
        <Route path="teams" element={<TeamsPage />} />
        <Route path="members" element={<MembersPage />} />
        <Route path="service-accounts" element={<ServiceAccountsPage />} />
        <Route path="service-accounts/:saId" element={<ServiceAccountDetailPage />} />
        <Route path="audit-log" element={<AuditLogPage />} />
        <Route path="settings" element={<OrgSettingsPage />} />
      </Route>

      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  );
}

import { Link, NavLink, Outlet, useNavigate, useParams } from "react-router-dom";

import { useAuth } from "../auth/AuthContext";
import { useOrganizations } from "../api/hooks/useTenancy";
import { useTheme } from "../theme";

const NAV_ITEMS = [
  { to: "", label: "Overview", end: true },
  { to: "projects", label: "Projects" },
  { to: "machines", label: "Machines" },
  { to: "networks-volumes", label: "Networks & volumes" },
  { to: "compose-files", label: "Compose files" },
  { to: "operations", label: "Operations" },
  { to: "variables", label: "Variables" },
  { to: "secret-mounts", label: "Secret mounts" },
  { to: "roles", label: "Roles" },
  { to: "role-bindings", label: "Role bindings" },
  { to: "teams", label: "Teams" },
  { to: "members", label: "Members" },
  { to: "service-accounts", label: "Service accounts" },
  { to: "audit-log", label: "Audit log" },
  { to: "settings", label: "Settings" },
];

export function OrgLayout() {
  const { orgId = "" } = useParams();
  const { data: orgs } = useOrganizations();
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  const [theme, toggleTheme] = useTheme();

  return (
    <div className="org-layout">
      <nav className="sidebar">
        <div className="sidebar-brand">
          <img src="/logo-horizontal.png" alt="Kaman Insurance" />
        </div>
        <div className="sidebar-org-switcher">
          <select
            value={orgId}
            onChange={(e) => navigate(`/orgs/${e.target.value}`)}
          >
            {orgs?.data.map((o) => (
              <option key={o.id} value={o.id}>
                {o.name}
              </option>
            ))}
          </select>
          {user?.is_platform_admin && (
            <Link to="/platform-admin" className="mono" style={{ display: "block", marginTop: 6, fontSize: 12 }}>
              Platform admin →
            </Link>
          )}
        </div>
        <div className="sidebar-nav">
          {NAV_ITEMS.map((item) => (
            <NavLink
              key={item.to}
              to={`/orgs/${orgId}/${item.to}`}
              end={item.end}
              className={({ isActive }) => (isActive ? "active" : "")}
            >
              {item.label}
            </NavLink>
          ))}
        </div>
        <div className="sidebar-user">
          <div>{user?.username ?? "…"}</div>
          <div className="sidebar-user-actions">
            <button className="secondary theme-toggle" onClick={toggleTheme}>
              {theme === "dark" ? "Light mode" : "Dark mode"}
            </button>
            <button className="secondary" onClick={logout}>
              Sign out
            </button>
          </div>
        </div>
      </nav>
      <div className="content">
        <Outlet />
      </div>
    </div>
  );
}

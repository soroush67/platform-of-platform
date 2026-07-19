import { useState } from "react";
import { NavLink, Outlet, useNavigate, useParams } from "react-router-dom";

import { useAuth } from "../auth/AuthContext";
import { useOrganizations } from "../api/hooks/useTenancy";
import { useTheme } from "../theme";

type NavItem = { to: string; label: string; end?: boolean };
type NavGroup = { key: string; label: string; items: NavItem[] };

// Top-level items stay ungrouped - Overview is the landing page,
// Projects is the primary day-to-day workflow entry point.
const TOP_LEVEL_ITEMS: NavItem[] = [
  { to: "", label: "Overview", end: true },
  { to: "projects", label: "Projects" },
];

// Everything else grouped by purpose (operator's own ask: stop menu
// sprawl, make navigation purposeful) - Fleet mirrors internal/fleet's
// own real bounded-context grouping of these exact 4 resources, not an
// invented seam.
const NAV_GROUPS: NavGroup[] = [
  {
    key: "fleet",
    label: "Fleet",
    items: [
      { to: "machines", label: "Machines" },
      { to: "networks-volumes", label: "Networks & volumes" },
      { to: "compose-files", label: "Compose files" },
      { to: "operations", label: "Operations" },
    ],
  },
  {
    key: "access-control",
    label: "Access Control",
    items: [
      { to: "roles", label: "Roles" },
      { to: "role-bindings", label: "Role bindings" },
      { to: "teams", label: "Teams" },
      { to: "members", label: "Members" },
      { to: "service-accounts", label: "Service accounts" },
    ],
  },
  {
    key: "platform",
    label: "Platform",
    items: [
      { to: "variables", label: "Variables" },
      { to: "secret-mounts", label: "Secret mounts" },
      { to: "audit-log", label: "Audit log" },
      { to: "settings", label: "Settings" },
    ],
  },
];

export function OrgLayout() {
  const { orgId = "" } = useParams();
  const { data: orgs } = useOrganizations();
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  const [theme, toggleTheme] = useTheme();
  // Every group starts expanded - grouping declutters the list once an
  // operator knows where things live, it shouldn't hide anything on
  // first load.
  const [openGroups, setOpenGroups] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(NAV_GROUPS.map((g) => [g.key, true])),
  );

  function toggleGroup(key: string) {
    setOpenGroups((prev) => ({ ...prev, [key]: !prev[key] }));
  }

  const topLevelItems = user?.is_platform_admin
    ? [{ to: "platform-admin", label: "Organizations" }, ...TOP_LEVEL_ITEMS]
    : TOP_LEVEL_ITEMS;

  return (
    <div className="org-layout">
      <nav className="sidebar">
        <div className="sidebar-brand">
          <img src="/logo-horizontal.png" alt="Kaman Insurance" />
        </div>
        <div className="sidebar-org-switcher">
          <select value={orgId} onChange={(e) => navigate(`/orgs/${e.target.value}`)}>
            {orgs?.data.map((o) => (
              <option key={o.id} value={o.id}>
                {o.name}
              </option>
            ))}
          </select>
        </div>
        <div className="sidebar-nav">
          {topLevelItems.map((item) => (
            <NavLink
              key={item.to}
              to={`/orgs/${orgId}/${item.to}`}
              end={item.end}
              className={({ isActive }) => (isActive ? "active" : "")}
            >
              {item.label}
            </NavLink>
          ))}
          {NAV_GROUPS.map((group) => (
            <div key={group.key} className="sidebar-nav-group">
              <button type="button" className="sidebar-nav-group-header" onClick={() => toggleGroup(group.key)}>
                <span>{group.label}</span>
                <span className="sidebar-nav-group-caret">{openGroups[group.key] ? "▾" : "▸"}</span>
              </button>
              {openGroups[group.key] && (
                <div className="sidebar-nav-group-items">
                  {group.items.map((item) => (
                    <NavLink
                      key={item.to}
                      to={`/orgs/${orgId}/${item.to}`}
                      className={({ isActive }) => (isActive ? "active" : "")}
                    >
                      {item.label}
                    </NavLink>
                  ))}
                </div>
              )}
            </div>
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

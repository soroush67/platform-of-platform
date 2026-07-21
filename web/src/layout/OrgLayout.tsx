import { useState } from "react";
import { NavLink, Outlet, useNavigate, useParams } from "react-router-dom";

import { useAuth } from "../auth/AuthContext";
import { useOrganizations } from "../api/hooks/useTenancy";
import { useTheme } from "../theme";

type NavItem = { to: string; label: string; end?: boolean };
// A group's own items can either be a plain leaf link or a real
// sub-group (its own expand/collapse, own nested links) - "docker-
// compose" below is the first real user of the sub-group shape, kept
// general rather than a one-off special case so a future compose-only
// page (or another engine growing its own sub-menu) has a natural home.
type NavSubGroup = { key: string; label: string; children: NavItem[] };
type NavEntry = NavItem | NavSubGroup;
type NavGroup = { key: string; label: string; items: NavEntry[] };

function isNavSubGroup(entry: NavEntry): entry is NavSubGroup {
  return "children" in entry;
}

// Top-level items stay ungrouped - Overview is the landing page,
// Projects is the primary day-to-day workflow entry point.
const TOP_LEVEL_ITEMS: NavItem[] = [
  { to: "", label: "Overview", end: true },
  { to: "projects", label: "Projects" },
];

// Everything else grouped by purpose (operator's own ask: stop menu
// sprawl, make navigation purposeful) - Fleet mirrors internal/fleet's
// own real bounded-context grouping of these exact 4 resources, not an
// invented seam. "Compose files" nests under a real "docker-compose"
// sub-group (operator's own explicit ask) - Fleet's ComposeFile system
// is now the one real way to run docker-compose (Workspace's own
// "compose" engine was retired as a creatable option), so the nav makes
// that relationship visible rather than sitting as a flat sibling of
// Machines/Operations.
const NAV_GROUPS: NavGroup[] = [
  {
    key: "fleet",
    label: "Fleet",
    items: [
      { to: "machines", label: "Machines" },
      { to: "networks-volumes", label: "Networks & volumes" },
      { key: "docker-compose", label: "docker-compose", children: [{ to: "compose-files", label: "Compose files" }] },
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
  // Every group (and sub-group) starts expanded - grouping declutters the
  // list once an operator knows where things live, it shouldn't hide
  // anything on first load. Sub-group keys share the same flat
  // openGroups record as top-level groups (each NavSubGroup's own `key`
  // is already unique across this static nav tree).
  const [openGroups, setOpenGroups] = useState<Record<string, boolean>>(() =>
    Object.fromEntries([
      ...NAV_GROUPS.map((g) => [g.key, true]),
      ...NAV_GROUPS.flatMap((g) => g.items).filter(isNavSubGroup).map((sg) => [sg.key, true]),
    ]),
  );

  function toggleGroup(key: string) {
    setOpenGroups((prev) => ({ ...prev, [key]: !prev[key] }));
  }

  // Overview stays first even for a platform admin (operator's own
  // ask) - Organizations (the platform-admin panel link) sits between
  // Overview and Projects, not ahead of both.
  const topLevelItems = user?.is_platform_admin
    ? [TOP_LEVEL_ITEMS[0], { to: "platform-admin", label: "Organizations" }, TOP_LEVEL_ITEMS[1]]
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
                  {group.items.map((item) =>
                    isNavSubGroup(item) ? (
                      <div key={item.key} className="sidebar-nav-group">
                        <button
                          type="button"
                          className="sidebar-nav-group-header"
                          onClick={() => toggleGroup(item.key)}
                        >
                          <span>{item.label}</span>
                          <span className="sidebar-nav-group-caret">{openGroups[item.key] ? "▾" : "▸"}</span>
                        </button>
                        {openGroups[item.key] && (
                          <div className="sidebar-nav-group-items">
                            {item.children.map((child) => (
                              <NavLink
                                key={child.to}
                                to={`/orgs/${orgId}/${child.to}`}
                                className={({ isActive }) => (isActive ? "active" : "")}
                              >
                                {child.label}
                              </NavLink>
                            ))}
                          </div>
                        )}
                      </div>
                    ) : (
                      <NavLink
                        key={item.to}
                        to={`/orgs/${orgId}/${item.to}`}
                        className={({ isActive }) => (isActive ? "active" : "")}
                      >
                        {item.label}
                      </NavLink>
                    ),
                  )}
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

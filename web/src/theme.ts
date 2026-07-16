import { useEffect, useState } from "react";

export type Theme = "dark" | "light";

const THEME_KEY = "pop_theme";

// getSystemTheme mirrors the same prefers-color-scheme check index.html's
// own inline no-flash script already made before React mounted - kept in
// sync deliberately (see that script's own comment).
function getSystemTheme(): Theme {
  return window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
}

export function getStoredTheme(): Theme {
  const stored = localStorage.getItem(THEME_KEY);
  if (stored === "dark" || stored === "light") return stored;
  return getSystemTheme();
}

export function applyTheme(theme: Theme) {
  document.documentElement.dataset.theme = theme;
  localStorage.setItem(THEME_KEY, theme);
}

// useTheme is a thin, no-dependency hook (plain useState + effect) - this
// project deliberately doesn't add a global state library for one boolean
// toggle, matching AuthContext's own "plain React state for UI-only
// concerns" precedent (docs/architecture/16-web-ui.md).
export function useTheme(): [Theme, () => void] {
  const [theme, setThemeState] = useState<Theme>(() => getStoredTheme());

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  function toggle() {
    setThemeState((prev) => (prev === "dark" ? "light" : "dark"));
  }

  return [theme, toggle];
}

"use client";

import * as React from "react";

export type Theme = "dark" | "light";

const STORAGE_KEY = "sf-theme";

type ThemeContextValue = {
  theme: Theme;
  setTheme: (theme: Theme) => void;
  toggle: () => void;
  mounted: boolean;
};

const ThemeContext = React.createContext<ThemeContextValue | null>(null);

/**
 * Theme state.
 *
 * The initial value is written by an inline script in <head> before paint, so
 * there is no flash and no hydration mismatch: this provider only reads back
 * what the script already decided.
 */
export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const [theme, setThemeState] = React.useState<Theme>("dark");
  const [mounted, setMounted] = React.useState(false);

  React.useEffect(() => {
    const current = document.documentElement.dataset.theme;
    setThemeState(current === "light" ? "light" : "dark");
    setMounted(true);
  }, []);

  const apply = React.useCallback((next: Theme) => {
    document.documentElement.dataset.theme = next;
    try {
      localStorage.setItem(STORAGE_KEY, next);
    } catch {
      // Private mode: the theme still applies for this session.
    }
    setThemeState(next);
  }, []);

  const toggle = React.useCallback(() => {
    apply(document.documentElement.dataset.theme === "light" ? "dark" : "light");
  }, [apply]);

  const value = React.useMemo(
    () => ({ theme, setTheme: apply, toggle, mounted }),
    [theme, apply, toggle, mounted],
  );

  useThemeColorMeta();

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeContextValue {
  const context = React.useContext(ThemeContext);
  if (!context) throw new Error("useTheme must be used inside ThemeProvider");
  return context;
}

/**
 * Keeps the browser chrome (the address bar on mobile) in step with the page
 * surface. The colour is read from the token rather than written as a literal,
 * so tokens.css stays the only file that defines a colour.
 */
export function useThemeColorMeta(): void {
  React.useEffect(() => {
    const surface = getComputedStyle(document.documentElement)
      .getPropertyValue("--surface-page")
      .trim();
    if (!surface) return;

    let meta = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]');
    if (!meta) {
      meta = document.createElement("meta");
      meta.name = "theme-color";
      document.head.appendChild(meta);
    }
    meta.content = surface;
  }, []);
}

/**
 * Runs before paint. Reads the stored preference, falls back to the OS setting,
 * and writes the attribute the CSS keys off. Kept as a string so it can be
 * inlined in <head> with no bundling.
 */
export const themeScript = `
(function () {
  try {
    var stored = localStorage.getItem("${STORAGE_KEY}");
    var theme = stored === "light" || stored === "dark"
      ? stored
      : (window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark");
    document.documentElement.dataset.theme = theme;
  } catch (e) {
    document.documentElement.dataset.theme = "dark";
  }
})();
`;
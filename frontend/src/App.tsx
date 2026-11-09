import { useEffect, useState } from "react";
import { SessionProvider, useSession } from "./state/session";
import { destinations } from "./components/AppShell";
import { HomePage } from "./pages/HomePage";
import { ExplorePage } from "./pages/ExplorePage";
import { MessagesPage } from "./pages/MessagesPage";
import { NotificationsPage } from "./pages/NotificationsPage";
import { StatesPage } from "./pages/StatesPage";

/* ==========================================================================
   App root.
   --------------------------------------------------------------------------
   Routing is hash-based so the built app can be served as static files by
   nginx without per-route configuration, and every link in the UI is a real
   anchor: middle-clickable, openable in a new tab, and readable by a screen
   reader as a link. Each page renders its own document title.
   ========================================================================== */

const ROUTES = ["home", "explore", "messages", "notifications", "states"] as const;
type Route = (typeof ROUTES)[number];

function routeFromHash(hash: string): Route {
  const path = hash.replace(/^#\/?/, "").split(/[?#]/)[0];
  return (ROUTES as readonly string[]).includes(path) ? (path as Route) : "home";
}

function Router() {
  const [route, setRoute] = useState<Route>(() => routeFromHash(window.location.hash));
  const { signedIn } = useSession();

  useEffect(() => {
    const onHashChange = () => setRoute(routeFromHash(window.location.hash));
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  useEffect(() => {
    const active = destinations.find((item) => item.id === route);
    document.title = `${active?.label ?? "Home"} · Home / X`;
  }, [route]);

  if (!signedIn && route !== "home") {
    // A signed-out visitor can still look around: the pages render demo data
    // with a banner explaining that, rather than dead-ending on a login wall.
  }

  switch (route) {
    case "explore":
      return <ExplorePage />;
    case "messages":
      return <MessagesPage />;
    case "notifications":
      return <NotificationsPage />;
    case "states":
      return <StatesPage />;
    default:
      return <HomePage />;
  }
}

export default function App() {
  return (
    <SessionProvider>
      <Router />
    </SessionProvider>
  );
}

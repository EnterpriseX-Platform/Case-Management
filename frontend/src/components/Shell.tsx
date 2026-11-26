"use client";
// UI shell: a workspace switcher in the header (Workspace, Operations,
// Studio, Admin) and one short side navigation per workspace. People only
// see the workspaces their roles open.
import { currentLang, setLang, t } from "@/lib/i18n";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { api, Incident, Notification, Work } from "@/lib/api";
import { signOut } from "@/lib/oidc";
import { has, useSession } from "@/lib/useSession";
import type { Session } from "@/lib/api";

type Item = { href: string; label: string; count?: "work" | "incidents"; exact?: boolean };
type WS = { key: string; name: string; who: string; home: string; roles?: string[]; match: (p: string) => boolean; items: Item[] };

export const WORKSPACES: WS[] = [
  {
    key: "workspace", name: "Workspace", who: "Submit cases and do your work", home: "/home",
    match: (p) => /^\/(home|work|cases|profile|notifications)(\/|$)/.test(p) || p === "/",
    items: [
      { href: "/home", label: "Home" }, { href: "/work", label: "My work", count: "work" },
      { href: "/cases", label: "Cases", exact: true }, { href: "/cases/new", label: "New case" },
    ],
  },
  {
    key: "operations", name: "Operations", who: "Follow up and manage the work", home: "/supervisor", roles: ["cm:supervisor"],
    match: (p) => /^\/(supervisor|ops|incidents)(\/|$)/.test(p),
    items: [
      { href: "/supervisor", label: "Overview" }, { href: "/ops/queues", label: "Work queues" },
      { href: "/incidents", label: "Incidents", count: "incidents" },
    ],
  },
  {
    key: "studio", name: "Studio", who: "Build and publish apps", home: "/studio", roles: ["cm:designer", "cm:approver"],
    match: (p) => /^\/(studio|designer)(\/|$)/.test(p),
    items: [
      { href: "/studio", label: "Apps", exact: true }, { href: "/designer/forms", label: "Forms" }, { href: "/designer/screens", label: "Screens" },
    ],
  },
  {
    key: "admin", name: "Admin", who: "Users, settings and data", home: "/admin/users", roles: ["cm:admin", "cm:supervisor"],
    match: (p) => /^\/admin(\/|$)/.test(p),
    items: [
      { href: "/admin/users", label: "Users & queues" }, { href: "/admin/calendar", label: "Business calendar" },
      { href: "/admin", label: "Import", exact: true }, { href: "/admin/migration", label: "Version migration" }, { href: "/admin/audit", label: "Audit log" },
    ],
  },
];

export const allowed = (w: WS, s: Session) => !w.roles || w.roles.some((r) => has(s, r));

function Bell() {
  const [n, setN] = useState(0);
  useEffect(() => {
    let dead = false;
    const f = () => api.get<Notification[]>("/api/notifications").then((x) => !dead && setN((x ?? []).filter((y) => !y.read).length)).catch(() => {});
    f();
    const id = setInterval(f, 30000);
    return () => { dead = true; clearInterval(id); };
  }, []);
  return <Link href="/notifications" className="hdr-link">{t("Notifications")}{n ? ` (${n})` : ""}</Link>;
}

function useCounts(ws: string) {
  const [c, setC] = useState<{ work?: number; incidents?: number }>({});
  const path = usePathname();
  useEffect(() => {
    if (ws === "workspace") api.get<Work[]>("/api/work").then((x) => setC({ work: (x ?? []).length })).catch(() => {});
    if (ws === "operations") api.get<Incident[]>("/api/incidents?status=open").then((x) => setC({ incidents: (x ?? []).length })).catch(() => {});
  }, [ws, path]);
  return c;
}

export default function Shell({ children }: { children: React.ReactNode }) {
  const s = useSession();
  const path = usePathname();
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (s === null && path !== "/login" && !path.startsWith("/auth/")) router.replace("/login");
  }, [s, path, router]);
  useEffect(() => {
    const h = (e: MouseEvent) => menuRef.current && !menuRef.current.contains(e.target as Node) && setOpen(false);
    document.addEventListener("mousedown", h);
    return () => document.removeEventListener("mousedown", h);
  }, []);
  useEffect(() => setOpen(false), [path]);
  const ws = WORKSPACES.find((w) => w.match(path)) ?? WORKSPACES[0];
  const counts = useCounts(ws.key);

  if (path === "/login" || path.startsWith("/auth/")) return <>{children}</>;
  if (!s) return null;
  const mine = WORKSPACES.filter((w) => allowed(w, s));
  // inside an app the studio uses its own tabs and the full width
  const inApp = /^\/studio\/[^/]+\/[^/]+/.test(path);
  const active = (i: Item) => (i.exact ? path === i.href : path === i.href || path.startsWith(i.href + "/"));

  return (
    <div className={"shell" + (inApp ? " shell-wide" : "")}>
      <header className="hdr">
        <div className="sw-wrap" ref={menuRef}>
          <button className="switch" aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen(!open)} disabled={mine.length < 2}>
            {mine.length > 1 && <span className="sw-grid" aria-hidden>{Array.from({ length: 9 }, (_, i) => <i key={i} />)}</span>}
            Case Management <span className="sw-name">{t(ws.name)}</span>
          </button>
          {open && (
            <div className="sw-menu" role="menu">
              {mine.map((w) => (
                <Link key={w.key} href={w.home} role="menuitem" className={w.key === ws.key ? "on" : ""}>
                  <b>{t(w.name)}</b><span>{t(w.who)}</span>
                </Link>
              ))}
            </div>
          )}
        </div>
        <div className="hdr-sp" />
        <div className="hdr-user">
          <button onClick={() => setLang(currentLang() === "th" ? "en" : "th")}>{currentLang() === "th" ? "EN" : "ไทย"}</button>
          <Bell />
          <Link href="/profile" className="hdr-link">{s.name}</Link>
          <button onClick={() => signOut()}>{t("Sign out")}</button>
        </div>
      </header>
      {!inApp && (
        <aside className="side">
          <div className="side-h">{t(ws.name)}</div>
          {ws.items.map((i) => (
            <Link key={i.href} href={i.href} className={"side-a" + (active(i) ? " on" : "")}>
              <span>{t(i.label)}</span>
              {i.count && counts[i.count] ? <span className="side-n">{counts[i.count]}</span> : null}
            </Link>
          ))}
        </aside>
      )}
      <main className="main">{children}</main>
    </div>
  );
}

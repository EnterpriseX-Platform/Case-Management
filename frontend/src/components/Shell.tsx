"use client";
import { currentLang, setLang, t } from "@/lib/i18n";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { api, Notification } from "@/lib/api";
import { signOut } from "@/lib/oidc";
import { has, useSession } from "@/lib/useSession";

type Item = { href: string; label: string; role?: string };
const groups: { title: string; items: Item[] }[] = [
  { title: t("My work"), items: [
    { href: "/home", label: t("Home") },
    { href: "/work", label: t("Inbox") },
    { href: "/cases", label: t("All cases") },
    { href: "/cases/new", label: t("New case") },
  ] },
  { title: t("Monitoring"), items: [
    { href: "/supervisor", label: t("Overview & queues"), role: "cm:supervisor" },
    { href: "/incidents", label: t("Incidents"), role: "cm:supervisor" },
  ] },
  { title: t("Design"), items: [
    { href: "/studio", label: t("App Studio"), role: "cm:designer" },
    { href: "/designer", label: t("Case types & flows"), role: "cm:designer" },
    { href: "/designer/forms", label: t("Forms"), role: "cm:designer" },
    { href: "/designer/screens", label: t("Screens"), role: "cm:designer" },
  ] },
  { title: t("Administrator"), items: [
    { href: "/admin", label: t("Import"), role: "cm:admin" },
    { href: "/admin/migration", label: t("Version migration"), role: "cm:admin" },
    { href: "/admin/users", label: t("Users & queues"), role: "cm:supervisor" },
    { href: "/admin/calendar", label: t("Business calendar"), role: "cm:admin" },
    { href: "/admin/audit", label: t("Audit log"), role: "cm:admin" },
  ] },
];

function Bell() {
  const [n, setN] = useState(0);
  useEffect(() => {
    let dead = false;
    const f = () => api.get<Notification[]>("/api/notifications").then((x) => !dead && setN((x ?? []).filter((y) => !y.read).length)).catch(() => {});
    f();
    const id = setInterval(f, 30000);
    return () => { dead = true; clearInterval(id); };
  }, []);
  return <Link href="/notifications" className="hdr-link" title={t("Notifications")}>{t("Notifications")}{n ? ` (${n})` : ""}</Link>;
}

export default function Shell({ children }: { children: React.ReactNode }) {
  const s = useSession();
  const path = usePathname();
  const router = useRouter();
  useEffect(() => {
    if (s === null && path !== "/login" && !path.startsWith("/auth/")) router.replace("/login");
  }, [s, path, router]);

  if (path === "/login" || path.startsWith("/auth/")) return <>{children}</>;
  if (!s) return null;
  const canDesign = has(s, "cm:designer") || has(s, "cm:approver");
  return (
    <div className="shell">
      <header className="hdr">
        <Link href="/" className="hdr-name"><span>EnterpriseX</span>Case Management</Link>
        <div className="hdr-sp" />
        <div className="hdr-user">
          <button onClick={() => setLang(currentLang() === "th" ? "en" : "th")} title={t("Language")}>{currentLang() === "th" ? "EN" : "ไทย"}</button>
          <Bell />
          <Link href="/profile" className="hdr-link">{s.name}</Link>
          <button onClick={() => signOut()}>{t("Sign out")}</button>
        </div>
      </header>
      <aside className="side">
        {groups.map((g) => {
          const items = g.items.filter((i) => !i.role || has(s, i.role) || (i.role === "cm:designer" && canDesign));
          if (!items.length) return null;
          return (
            <nav key={g.title}>
              <div className="side-h">{g.title}</div>
              {items.map((i) => (
                <Link key={i.href} href={i.href}
                  className={"side-a" + (path === i.href || (i.href !== "/cases" && i.href !== "/designer" && path.startsWith(i.href + "/")) ? " on" : "")}>
                  {i.label}
                </Link>
              ))}
            </nav>
          );
        })}
      </aside>
      <main className="main">{children}</main>
    </div>
  );
}

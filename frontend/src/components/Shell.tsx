"use client";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect } from "react";
import { setSession } from "@/lib/api";
import { has, useSession } from "@/lib/useSession";

type Item = { href: string; label: string; role?: string };
const groups: { title: string; items: Item[] }[] = [
  { title: "งานของฉัน", items: [
    { href: "/work", label: "กล่องงาน" },
    { href: "/cases", label: "เรื่องทั้งหมด" },
    { href: "/cases/new", label: "ยื่นเรื่องใหม่" },
  ] },
  { title: "ติดตามงาน", items: [
    { href: "/supervisor", label: "ภาพรวมและคิวงาน", role: "cm:supervisor" },
  ] },
  { title: "ออกแบบ", items: [
    { href: "/designer", label: "ประเภทเรื่องและ flow", role: "cm:designer" },
    { href: "/designer/forms", label: "ฟอร์ม", role: "cm:designer" },
    { href: "/designer/screens", label: "หน้าจอ", role: "cm:designer" },
  ] },
  { title: "ผู้ดูแลระบบ", items: [
    { href: "/admin", label: "นำเข้าข้อมูล", role: "cm:admin" },
    { href: "/admin/migration", label: "ย้ายเรื่องข้ามเวอร์ชัน", role: "cm:admin" },
    { href: "/admin/audit", label: "ประวัติการใช้งาน", role: "cm:admin" },
  ] },
];

export default function Shell({ children }: { children: React.ReactNode }) {
  const s = useSession();
  const path = usePathname();
  const router = useRouter();
  useEffect(() => {
    if (s === null && path !== "/login") router.replace("/login");
  }, [s, path, router]);

  if (path === "/login") return <>{children}</>;
  if (!s) return null;
  const canDesign = has(s, "cm:designer") || has(s, "cm:approver");
  return (
    <div className="shell">
      <header className="hdr">
        <Link href="/" className="hdr-name"><span>EnterpriseX</span>Case Management</Link>
        <div className="hdr-sp" />
        <div className="hdr-user">
          <span>{s.name}</span>
          <button onClick={() => { setSession(null); router.replace("/login"); }}>ออกจากระบบ</button>
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

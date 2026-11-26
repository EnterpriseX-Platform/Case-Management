"use client";
import Link from "next/link";
import { th } from "@/lib/format";

export function Page({ title, actions, children, crumb }: { title: string; actions?: React.ReactNode; children: React.ReactNode; crumb?: { href: string; label: string } }) {
  return (
    <div className="page">
      {crumb && <div className="crumb"><Link href={crumb.href}>{crumb.label}</Link></div>}
      <div className="page-head"><h1>{title}</h1><div className="actions">{actions}</div></div>
      {children}
    </div>
  );
}

export function Status({ s }: { s?: string }) {
  return <span className={"st st-" + (s ?? "")}>{th(s)}</span>;
}

export function Err({ e }: { e?: string }) {
  return e ? <div className="err">{e}</div> : null;
}

export function Field({ label, children, hint }: { label: string; children: React.ReactNode; hint?: string }) {
  return (
    <label className="fld">
      <span className="fld-l">{label}</span>
      {children}
      {hint && <span className="muted small">{hint}</span>}
    </label>
  );
}

export function Empty({ children }: { children: React.ReactNode }) {
  return <div className="empty muted">{children}</div>;
}

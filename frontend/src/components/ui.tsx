"use client";
import { th } from "@/lib/format";

export function Page({ title, actions, children }: { title: string; actions?: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="page">
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

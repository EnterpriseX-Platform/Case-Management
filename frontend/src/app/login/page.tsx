"use client";
// Development sign-in. In production identities come from the IAM/OIDC
// gateway; this page lets testers act as any role.
import { t } from "@/lib/i18n";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { setSession } from "@/lib/api";
import Dropdown from "@/components/Dropdown";
import { Field } from "@/components/ui";
import { oidc, oidcEnabled, signIn } from "@/lib/oidc";

const personas = [
  { id: "admin", name: "Administrator", roles: ["cm:admin"] },
  { id: "designer", name: "Flow designer", roles: ["cm:designer"] },
  { id: "approver", name: "Version approver", roles: ["cm:approver"] },
  { id: "supervisor", name: "Supervisor", roles: ["cm:supervisor"] },
  { id: "requester", name: "Requester", roles: ["requester"] },
  { id: "reviewer", name: "Reviewer", roles: ["reviewer"] },
  { id: "manager", name: "Approver", roles: ["approver"] },
  { id: "buyer", name: "Procurement officer", roles: ["procurement"] },
  { id: "finance", name: "Finance officer", roles: ["finance"] },
];

export default function Login() {
  const r = useRouter();
  const [id, setId] = useState("");
  const [name, setName] = useState("");
  const [roles, setRoles] = useState<string[]>([]);
  const all = Array.from(new Set(personas.flatMap((p) => p.roles)));
  return (
    <div className="login">
      <h1>Case Management</h1>
      {oidcEnabled() && (
        <div className="bar"><button className="btn" onClick={() => signIn("/")}>{t("Sign in with SSO")}</button></div>
      )}
      {(!oidcEnabled() || oidc.devLogin) && <>
      <div className="login-grid">
        {personas.map((p) => (
          <button key={p.id} className="persona" onClick={() => { setSession(p); r.replace("/"); }}>
            <div>{p.name}</div><div className="muted small">{p.roles.join(", ")}</div>
          </button>
        ))}
      </div>
      <h3>{t("Custom")}</h3>
      <div className="row3">
        <Field label={t("User ID")}><input value={id} onChange={(e) => setId(e.target.value)} /></Field>
        <Field label={t("Name")}><input value={name} onChange={(e) => setName(e.target.value)} /></Field>
        <Field label={t("Role")}><Dropdown multiple allowCustom options={all.map((x) => ({ value: x, label: x }))} value={roles} onChange={setRoles} /></Field>
      </div>
      <button className="btn" disabled={!id || !roles.length} onClick={() => { setSession({ id, name: name || id, roles }); r.replace("/"); }}>{t("Sign in")}</button>
      </>}
    </div>
  );
}

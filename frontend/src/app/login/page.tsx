"use client";
// Development sign-in. In production identities come from the IAM/OIDC
// gateway; this page lets testers act as any role.
import { useRouter } from "next/navigation";
import { useState } from "react";
import { setSession } from "@/lib/api";
import Dropdown from "@/components/Dropdown";
import { Field } from "@/components/ui";

const personas = [
  { id: "admin", name: "ผู้ดูแลระบบ", roles: ["cm:admin"] },
  { id: "designer", name: "ผู้ออกแบบ flow", roles: ["cm:designer"] },
  { id: "approver", name: "ผู้อนุมัติเวอร์ชัน", roles: ["cm:approver"] },
  { id: "supervisor", name: "หัวหน้างาน", roles: ["cm:supervisor"] },
  { id: "requester", name: "ผู้ยื่นคำขอ", roles: ["requester"] },
  { id: "reviewer", name: "หน่วยงานให้ความเห็น", roles: ["reviewer"] },
  { id: "manager", name: "ผู้พิจารณาอนุมัติ", roles: ["approver"] },
  { id: "buyer", name: "เจ้าหน้าที่จัดซื้อ", roles: ["procurement"] },
  { id: "finance", name: "เจ้าหน้าที่การเงิน", roles: ["finance"] },
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
      <div className="login-grid">
        {personas.map((p) => (
          <button key={p.id} className="persona" onClick={() => { setSession(p); r.replace("/"); }}>
            <div>{p.name}</div><div className="muted small">{p.roles.join(", ")}</div>
          </button>
        ))}
      </div>
      <h3>กำหนดเอง</h3>
      <div className="row3">
        <Field label="รหัสผู้ใช้"><input value={id} onChange={(e) => setId(e.target.value)} /></Field>
        <Field label="ชื่อ"><input value={name} onChange={(e) => setName(e.target.value)} /></Field>
        <Field label="บทบาท"><Dropdown multiple allowCustom options={all.map((x) => ({ value: x, label: x }))} value={roles} onChange={setRoles} /></Field>
      </div>
      <button className="btn" disabled={!id || !roles.length} onClick={() => { setSession({ id, name: name || id, roles }); r.replace("/"); }}>เข้าใช้งาน</button>
    </div>
  );
}

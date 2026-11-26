"use client";
// Users (registered on first sign-in), their skills, capacity and absence,
// and work queues with members and supervisors.
import { useEffect, useState } from "react";
import { api, Queue, UserProfile } from "@/lib/api";
import { dt } from "@/lib/format";
import { t } from "@/lib/i18n";
import { useLoad } from "@/lib/useLoad";
import Dropdown from "@/components/Dropdown";
import { Err, Field } from "@/components/ui";
import { Page } from "@/components/ui";

export default function UsersQueues() {
  const { data: users, reload: reloadU } = useLoad(() => api.get<UserProfile[]>("/api/users"));
  const { data: queues, reload: reloadQ } = useLoad(() => api.get<Queue[]>("/api/queues"));
  const [tab, setTab] = useState<"users" | "queues">("users");
  const [edit, setEdit] = useState<Record<string, UserProfile>>({});
  const [q, setQ] = useState<Queue | null>(null);
  const [e, setE] = useState<string>();
  const [msg, setMsg] = useState<string>();
  useEffect(() => { if (users) setEdit(Object.fromEntries(users.map((u) => [u.id, u]))); }, [users]);
  const uo = (users ?? []).map((u) => ({ value: u.id, label: u.name || u.id, hint: u.id }));
  const roles = Array.from(new Set((users ?? []).flatMap((u) => u.roles ?? []))).map((r) => ({ value: r, label: r }));
  const allSkills = Array.from(new Set((users ?? []).flatMap((u) => u.skills ?? []))).map((r) => ({ value: r, label: r }));
  const ok = (m: string) => { setMsg(m); setE(undefined); };
  const saveUser = (u: UserProfile) => api.put(`/api/users/${u.id}`, u).then(() => { ok(`${u.id}: ${t("Saved")}`); reloadU(); }).catch((x) => setE(x.message));
  const saveQueue = () => q && api.put(`/api/queues/${q.key}`, q).then(() => { ok(t("Saved")); setQ(null); reloadQ(); }).catch((x) => setE(x.message));
  return (
    <Page title={t("Users & queues")}>
      <div className="tabs">
        <button className={tab === "users" ? "on" : ""} onClick={() => setTab("users")}>{t("Users")} ({users?.length ?? 0})</button>
        <button className={tab === "queues" ? "on" : ""} onClick={() => setTab("queues")}>{t("Queues")} ({queues?.length ?? 0})</button>
      </div>
      <Err e={e} />{msg && <div className="ok">{msg}</div>}
      {tab === "users" && (
        <table className="tbl map">
          <thead><tr><th>{t("User")}</th><th>{t("Roles")}</th><th>{t("Skills")}</th><th>{t("Capacity")}</th><th>{t("Active")}</th><th>{t("Away / delegate")}</th><th>{t("Last seen")}</th><th /></tr></thead>
          <tbody>
            {Object.values(edit).map((u) => (
              <tr key={u.id}>
                <td>{u.name}<div className="small muted mono">{u.id}</div></td>
                <td className="small">{(u.roles ?? []).join(", ")}</td>
                <td><Dropdown multiple allowCustom options={allSkills} value={u.skills ?? []} onChange={(v) => setEdit({ ...edit, [u.id]: { ...u, skills: v } })} placeholder="—" /></td>
                <td><input className="sm" inputMode="numeric" value={u.capacity || ""} placeholder="∞" onChange={(x) => setEdit({ ...edit, [u.id]: { ...u, capacity: Number(x.target.value) || 0 } })} /></td>
                <td><button className={"check" + (u.active ? " on" : "")} onClick={() => setEdit({ ...edit, [u.id]: { ...u, active: !u.active } })}>{u.active ? "✓" : ""}</button></td>
                <td className="small">{u.ooo?.delegate ? `→ ${u.ooo.delegate}${u.ooo.to ? ` (${dt(u.ooo.to)})` : ""}` : "-"}</td>
                <td className="small nw">{dt(u.lastSeen)}</td>
                <td><button className="btn-s" onClick={() => saveUser(u)}>{t("Save")}</button></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {tab === "queues" && (
        <>
          <div className="bar" style={{ marginTop: 0, marginBottom: 16 }}><button className="btn" onClick={() => setQ({ key: "", name: "", members: [], roles: [], supervisors: [] })}>{t("New queue")}</button></div>
          {q && (
            <section className="card">
              <div className="row3">
                <Field label={t("Name")}><input value={q.name} onChange={(x) => setQ({ ...q, name: x.target.value, key: q.key || x.target.value.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") })} /></Field>
                <Field label={t("Key")}><input className="mono" value={q.key} onChange={(x) => setQ({ ...q, key: x.target.value.toLowerCase() })} /></Field>
                <Field label={t("Members by role")}><Dropdown multiple allowCustom options={roles} value={q.roles ?? []} onChange={(v) => setQ({ ...q, roles: v })} placeholder="—" /></Field>
                <Field label={t("Members")}><Dropdown multiple options={uo} value={q.members ?? []} onChange={(v) => setQ({ ...q, members: v })} placeholder="—" /></Field>
                <Field label={t("Supervisors")}><Dropdown multiple options={uo} value={q.supervisors ?? []} onChange={(v) => setQ({ ...q, supervisors: v })} placeholder="—" /></Field>
              </div>
              <div className="bar"><button className="btn" disabled={!q.key || !q.name} onClick={saveQueue}>{t("Save")}</button><button className="link" onClick={() => setQ(null)}>{t("Cancel")}</button></div>
            </section>
          )}
          <table className="tbl mt">
            <thead><tr><th>{t("Queue")}</th><th>{t("Members by role")}</th><th>{t("Members")}</th><th>{t("Supervisors")}</th><th /></tr></thead>
            <tbody>{(queues ?? []).map((x) => (
              <tr key={x.key}>
                <td>{x.name}<div className="small muted mono">{x.key}</div></td>
                <td className="small">{(x.roles ?? []).join(", ") || "-"}</td>
                <td className="small">{(x.members ?? []).join(", ") || "-"}</td>
                <td className="small">{(x.supervisors ?? []).join(", ") || "-"}</td>
                <td className="nw"><button className="link" onClick={() => setQ(x)}>{t("Edit")}</button>
                  <button className="link danger" onClick={() => confirm(t("Delete queue?")) && api.del(`/api/queues/${x.key}`).then(reloadQ).catch((y) => setE(y.message))}>{t("Remove")}</button></td>
              </tr>
            ))}</tbody>
          </table>
        </>
      )}
    </Page>
  );
}

"use client";
// My profile: e-mail for notifications and out-of-office delegation.
import { useEffect, useState } from "react";
import { api, UserProfile } from "@/lib/api";
import { t } from "@/lib/i18n";
import { useLoad } from "@/lib/useLoad";
import Dropdown from "@/components/Dropdown";
import { Err, Field, Page } from "@/components/ui";

const toLocal = (s?: string) => (s ? s.slice(0, 16) : "");
const fromLocal = (s: string) => (s ? new Date(s).toISOString() : undefined);

export default function Profile() {
  const { data } = useLoad(() => api.get<UserProfile>("/api/me/profile"));
  const { data: users } = useLoad(() => api.get<UserProfile[]>("/api/users"));
  const [u, setU] = useState<UserProfile>();
  const [e, setE] = useState<string>();
  const [msg, setMsg] = useState<string>();
  useEffect(() => { if (data) setU(data); }, [data]);
  if (!u) return null;
  const away = !!u.ooo?.delegate;
  const save = () => api.put<UserProfile>("/api/me/profile", u).then((x) => { setU(x); setMsg(t("Saved")); setE(undefined); }).catch((x) => setE(x.message));
  return (
    <Page title={t("My profile")} actions={<button className="btn" onClick={save}>{t("Save")}</button>}>
      <Err e={e} />{msg && <div className="ok">{msg}</div>}
      <section className="card narrow">
        <Field label={t("Name")}><div className="display">{u.name} <span className="muted small mono">{u.id}</span></div></Field>
        <Field label={t("Roles")}><div className="display small">{u.roles.join(", ")}</div></Field>
        <Field label={t("E-mail for notifications")}><input type="email" value={u.email ?? ""} onChange={(x) => setU({ ...u, email: x.target.value })} /></Field>
      </section>
      <section className="card narrow mt">
        <h3>{t("Out of office")}</h3>
        <div className="fld">
          <button className={"toggle" + (away ? " on" : "")} onClick={() => setU({ ...u, ooo: away ? undefined : { delegate: "" } })}><span className="toggle-track"><span className="toggle-knob" /></span>{away ? t("On") : t("Off")}</button>
        </div>
        {u.ooo && <>
          <Field label={t("Delegate new work to")}><Dropdown options={(users ?? []).filter((x) => x.id !== u.id).map((x) => ({ value: x.id, label: x.name || x.id, hint: x.id }))} value={u.ooo.delegate} onChange={(v) => setU({ ...u, ooo: { ...u.ooo, delegate: v } })} /></Field>
          <div className="row2">
            <Field label={t("From")}><input type="datetime-local" value={toLocal(u.ooo.from)} onChange={(x) => setU({ ...u, ooo: { ...u.ooo, from: fromLocal(x.target.value) } })} /></Field>
            <Field label={t("To")}><input type="datetime-local" value={toLocal(u.ooo.to)} onChange={(x) => setU({ ...u, ooo: { ...u.ooo, to: fromLocal(x.target.value) } })} /></Field>
          </div>
        </>}
      </section>
    </Page>
  );
}

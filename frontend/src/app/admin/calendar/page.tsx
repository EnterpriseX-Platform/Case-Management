"use client";
// Business calendar: SLA durations count working time only.
import { useEffect, useState } from "react";
import { api, Calendar } from "@/lib/api";
import { t } from "@/lib/i18n";
import { useLoad } from "@/lib/useLoad";
import { Err, Field, Page } from "@/components/ui";

const DAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

export default function CalendarPage() {
  const { data } = useLoad(() => api.get<{ calendar: Calendar }>("/api/settings"));
  const [c, setC] = useState<Calendar>({ enabled: false, days: [1, 2, 3, 4, 5], start: "08:30", end: "16:30", holidays: [], tz: "Asia/Bangkok" });
  const [hol, setHol] = useState("");
  const [e, setE] = useState<string>();
  const [msg, setMsg] = useState<string>();
  useEffect(() => { if (data?.calendar?.days?.length) setC({ ...data.calendar, holidays: data.calendar.holidays ?? [] }); }, [data]);
  const save = () => api.put("/api/settings", { calendar: c }).then(() => { setMsg(t("Saved")); setE(undefined); }).catch((x) => setE(x.message));
  return (
    <Page title={t("Business calendar")} actions={<button className="btn" onClick={save}>{t("Save")}</button>}>
      <Err e={e} />{msg && <div className="ok">{msg}</div>}
      <section className="card">
        <div className="fld"><span className="fld-l">{t("Count SLAs in working time")}</span>
          <button className={"toggle" + (c.enabled ? " on" : "")} onClick={() => setC({ ...c, enabled: !c.enabled })}><span className="toggle-track"><span className="toggle-knob" /></span>{c.enabled ? t("On") : t("Off")}</button></div>
        <div className="fld"><span className="fld-l">{t("Working days")}</span>
          <div className="radios">{DAYS.map((d, i) => (
            <button key={d} className={"radio" + (c.days.includes(i) ? " on" : "")} onClick={() => setC({ ...c, days: c.days.includes(i) ? c.days.filter((x) => x !== i) : [...c.days, i].sort() })}>{t(d)}</button>
          ))}</div></div>
        <div className="row3">
          <Field label={t("Opens")}><input type="time" value={c.start} onChange={(x) => setC({ ...c, start: x.target.value })} /></Field>
          <Field label={t("Closes")}><input type="time" value={c.end} onChange={(x) => setC({ ...c, end: x.target.value })} /></Field>
          <Field label={t("Time zone")}><input className="mono" value={c.tz ?? ""} onChange={(x) => setC({ ...c, tz: x.target.value })} /></Field>
        </div>
      </section>
      <section className="card mt">
        <h3>{t("Holidays")}</h3>
        <div className="row3">
          <input type="date" value={hol} onChange={(x) => setHol(x.target.value)} />
          <div><button className="btn-s" disabled={!hol} onClick={() => { setC({ ...c, holidays: Array.from(new Set([...(c.holidays ?? []), hol])).sort() }); setHol(""); }}>{t("Add")}</button></div>
        </div>
        <table className="tbl mt"><tbody>{(c.holidays ?? []).map((h) => (
          <tr key={h}><td>{new Date(h).toLocaleDateString("en-GB", { weekday: "short", day: "numeric", month: "short", year: "numeric" })}</td>
            <td className="r"><button className="link danger" onClick={() => setC({ ...c, holidays: (c.holidays ?? []).filter((x) => x !== h) })}>{t("Remove")}</button></td></tr>
        ))}</tbody></table>
      </section>
    </Page>
  );
}

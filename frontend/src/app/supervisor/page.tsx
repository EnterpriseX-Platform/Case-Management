"use client";
import { t } from "@/lib/i18n";
import Link from "next/link";
import { useState } from "react";
import { api, Incident, ProcessDef, Queue, UserProfile, Work } from "@/lib/api";
import { dt, loopLabel, overdue } from "@/lib/format";
import { useLoad } from "@/lib/useLoad";
import Dropdown from "@/components/Dropdown";
import { Err, Page, Status } from "@/components/ui";

type Dash = { casesByStatus: Record<string, number>; steps: { version: string; no: number; node: string; name: string; open: number; overdue: number }[]; workByRole: Record<string, number>; overdue: number; openWork: number };

export default function Supervisor() {
  const [proc, setProc] = useState("");
  const { data: procs } = useLoad(() => api.get<ProcessDef[]>("/api/processes"));
  const { data: d, error } = useLoad(() => api.get<Dash>(`/api/dashboard?process=${proc}`), [proc]);
  const { data: inc } = useLoad(() => api.get<Incident[]>("/api/incidents?status=open"));
  const { data: work, reload } = useLoad(() => api.get<Work[]>("/api/work?all=1"));
  const [e, setE] = useState<string>();
  const [msg, setMsg] = useState<string>();
  const [picked, setPicked] = useState<string[]>([]);
  const [target, setTarget] = useState("");
  const [dest, setDest] = useState("");
  const { data: users } = useLoad(() => api.get<UserProfile[]>("/api/users"));
  const { data: queues } = useLoad(() => api.get<Queue[]>("/api/queues"));
  const bulk = (op: string, amount = 0) => api.post<{ updated: number }>("/api/work/bulk", { ids: picked, op, target: op === "transfer" ? dest : target, amount })
    .then((r) => { setE(undefined); setMsg(`${r.updated} ${t("updated")}`); setPicked([]); reload(); }).catch((x) => setE(x.message));
  const [only, setOnly] = useState("");
  const roleOpts = Array.from(new Set((work ?? []).map((w) => w.role).filter((r): r is string => !!r && !r.startsWith("queue:")))).map((r) => ({ value: r, label: `${t("Role")}: ${r}` }));
  const list = (work ?? []).filter((w) => !only || (only === "late" ? overdue(w.dueAt) : w.role === only));
  return (
    <Page title={t("Overview")}>
      <div className="filters"><Dropdown placeholder={t("All case types")} value={proc} onChange={setProc} options={(procs ?? []).map((p) => ({ value: p.key, label: p.name }))} /></div>
      <Err e={error ?? e} />{msg && <div className="ok">{msg}</div>}
      {d && (d.overdue > 0 || (inc ?? []).length > 0) && (
        <div className="callout warn-l">
          <span>{t("Overdue")}: <b>{d.overdue}</b> · {t("Incidents")}: <b>{(inc ?? []).length}</b></span>
          <span className="acts">
            {d.overdue > 0 && <Link className="btn-s" href="/ops/queues">{t("Work queues")}</Link>}
            {(inc ?? []).length > 0 && <Link className="btn-s" href="/incidents">{t("Incidents")}</Link>}
          </span>
        </div>
      )}
      {d && (
        <div className="row2">
          <section className="card">
            <h3>{t("Open work by step")}</h3>
            <table className="tbl">
              <thead><tr><th>{t("Step")}</th><th>{t("Version")}</th><th className="r">{t("Open items")}</th><th className="r">{t("Overdue")}</th></tr></thead>
              <tbody>{d.steps.map((x) => <tr key={x.version + x.node}><td>{x.name}</td><td>v{x.no}</td><td className="r">{x.open}</td><td className={"r" + (x.overdue ? " late-t" : "")}>{x.overdue}</td></tr>)}</tbody>
            </table>
          </section>
          <section className="card">
            <h3>{t("Summary")}</h3>
            <table className="tbl"><tbody>
              {Object.entries(d.casesByStatus).map(([k, v]) => <tr key={k}><td><Status s={k} /></td><td className="r">{v} {t("cases")}</td></tr>)}
              <tr><td>{t("Open work")}</td><td className="r">{d.openWork}</td></tr>
              <tr><td>{t("Overdue work")}</td><td className="r late-t">{d.overdue}</td></tr>
              {Object.entries(d.workByRole).map(([k, v]) => <tr key={k}><td className="muted">{t("Queue")} {k.replace(/^queue:/, "")}</td><td className="r">{v}</td></tr>)}
            </tbody></table>
          </section>
        </div>
      )}

    </Page>
  );
}

"use client";
// Operations overview: what needs attention now, flow of cases over the
// period, SLA health, where cases are, and who carries the work.
import Link from "next/link";
import { useState } from "react";
import { api, ProcessDef } from "@/lib/api";
import { t } from "@/lib/i18n";
import { useLoad } from "@/lib/useLoad";
import Dropdown from "@/components/Dropdown";
import { GroupedBars, StackBar } from "@/components/Charts";
import { Err, Page } from "@/components/ui";

type Overview = {
  days: { day: string; started: number; completed: number }[]; running: number; completed: number; started: number;
  cycleP50Hours: number; cycleP90Hours: number; sla: Record<string, number>;
  byStage: { key: string; name: string; count: number }[] | null;
  workload: { user: string; name: string; open: number; overdue: number; away: boolean; done: number }[] | null;
  unassigned: number; incidents: number;
};

const hours = (h: number) => (h === 0 ? "-" : h < 24 ? `${h.toFixed(1)} ${t("h")}` : `${(h / 24).toFixed(1)} ${t("d")}`);
const dayLabel = (d: string) => new Date(d).toLocaleDateString("en-GB", { day: "numeric", month: "short" });

export default function Overview() {
  const [proc, setProc] = useState("");
  const [days, setDays] = useState(30);
  const { data: procs } = useLoad(() => api.get<ProcessDef[]>("/api/processes"));
  const { data: o, error } = useLoad(() => api.get<Overview>(`/api/monitor/overview?process=${proc}&days=${days}`), [proc, days]);
  return (
    <Page title={t("Overview")}>
      <div className="filters">
        <Dropdown placeholder={t("All case types")} value={proc} onChange={setProc} options={(procs ?? []).map((p) => ({ value: p.key, label: p.name }))} />
        <div className="seg">{[7, 30, 90].map((d) => <button key={d} className={days === d ? "on" : ""} onClick={() => setDays(d)}>{d} {t("days")}</button>)}</div>
      </div>
      <Err e={error} />
      {o && (o.sla.overdue > 0 || o.incidents > 0 || o.unassigned > 0) && (
        <div className={"callout" + (o.sla.overdue || o.incidents ? " warn-l" : "")}>
          <span>
            {t("Overdue")}: <b>{o.sla.overdue}</b> · {t("Incidents")}: <b>{o.incidents}</b> · {t("Waiting for someone to pick up")}: <b>{o.unassigned}</b>
          </span>
          <span className="acts">
            {o.sla.overdue > 0 && <Link className="btn-s" href="/ops/cases?sla=overdue">{t("Overdue cases")}</Link>}
            {o.incidents > 0 && <Link className="btn-s" href="/incidents">{t("Incidents")}</Link>}
            {o.unassigned > 0 && <Link className="btn-s" href="/ops/queues">{t("Work queues")}</Link>}
          </span>
        </div>
      )}
      {o && (
        <>
          <div className="cols">
            <section className="card">
              <h3>{t("Cases started and closed")}</h3>
              <GroupedBars data={o.days.map((d) => ({ x: d.day, values: { started: d.started, completed: d.completed } }))}
                series={[{ key: "started", label: t("Started"), color: "var(--series-1)" }, { key: "completed", label: t("Closed"), color: "var(--series-2)" }]}
                xLabel={dayLabel} />
            </section>
            <section className="card">
              <h3>{t("In the period")}</h3>
              <table className="tbl"><tbody>
                <tr><td>{t("Started")}</td><td className="r">{o.started}</td></tr>
                <tr><td>{t("Closed")}</td><td className="r">{o.completed}</td></tr>
                <tr><td>{t("Open now")}</td><td className="r">{o.running}</td></tr>
                <tr><td>{t("Time to close (median)")}</td><td className="r">{hours(o.cycleP50Hours)}</td></tr>
                <tr><td>{t("Time to close (9 in 10 within)")}</td><td className="r">{hours(o.cycleP90Hours)}</td></tr>
              </tbody></table>
            </section>
          </div>
          <div className="cols mt">
            <section className="card">
              <h3>{t("Open work against SLA")}</h3>
              <StackBar parts={[
                { key: "ontime", label: t("On time"), value: o.sla.ontime, color: "var(--status-good)" },
                { key: "atrisk", label: t("Past goal"), value: o.sla.atrisk, color: "var(--status-warn)" },
                { key: "overdue", label: t("Overdue"), value: o.sla.overdue, color: "var(--status-bad)" },
                { key: "none", label: t("No SLA"), value: o.sla.none, color: "var(--status-none)" },
              ]} />
              <h3 className="mt">{t("Open cases by stage")}</h3>
              <table className="tbl"><tbody>
                {(o.byStage ?? []).map((s) => <tr key={s.key}><td><Link href={`/ops/cases?stage=${s.key}`}>{s.name}</Link></td><td className="r">{s.count}</td></tr>)}
              </tbody></table>
            </section>
            <section className="card">
              <h3>{t("Who carries the work")}</h3>
              <table className="tbl">
                <thead><tr><th>{t("Person")}</th><th className="r">{t("Open")}</th><th className="r">{t("Overdue")}</th><th className="r">{t("Done")}</th></tr></thead>
                <tbody>{(o.workload ?? []).map((p) => (
                  <tr key={p.user}>
                    <td><Link href={`/ops/cases?assignee=${encodeURIComponent(p.user)}`}>{p.name}</Link>{p.away && <span className="small muted"> · {t("away")}</span>}</td>
                    <td className="r">{p.open}</td><td className={"r" + (p.overdue ? " late-t" : "")}>{p.overdue}</td><td className="r">{p.done}</td>
                  </tr>
                ))}</tbody>
              </table>
            </section>
          </div>
        </>
      )}
    </Page>
  );
}

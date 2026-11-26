"use client";
// Flow monitor: live load on each step of a version, average time spent,
// overdue work and incidents; the busiest steps are shaded. Click a step to
// see its cases.
import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { api, ProcessDef, Version } from "@/lib/api";
import { t } from "@/lib/i18n";
import { useLoad } from "@/lib/useLoad";
import Dropdown from "@/components/Dropdown";
import { MonitorViewer } from "@/components/Bpmn";
import { Err, Page } from "@/components/ui";

type NodeStat = { node: string; name: string; type: string; active: number; overdue: number; incidents: number; passed: number; avgHours: number; p90Hours: number };
type Stats = { versionId: string; bpmn: string; cases: number; nodes: NodeStat[] };
const hours = (h: number) => (!h ? "-" : h < 24 ? `${h.toFixed(1)} ${t("h")}` : `${(h / 24).toFixed(1)} ${t("d")}`);

export default function Monitor() {
  const { data: procs } = useLoad(() => api.get<ProcessDef[]>("/api/processes"));
  const [proc, setProc] = useState("");
  const { data: vers } = useLoad(() => (proc ? api.get<Version[]>(`/api/processes/${proc}/versions`) : Promise.resolve([] as Version[])), [proc]);
  const [ver, setVer] = useState("");
  const [days, setDays] = useState(30);
  const [sel, setSel] = useState<string>();
  useEffect(() => { if (!proc && procs?.length) setProc(procs[0].key); }, [procs, proc]);
  useEffect(() => {
    const live = (vers ?? []).filter((v) => v.status === "published" || v.status === "deprecated");
    setVer(live[0]?.id ?? vers?.[0]?.id ?? "");
    setSel(undefined);
  }, [vers]);
  const { data: st, error, reload } = useLoad(() => (ver ? api.get<Stats>(`/api/monitor/process/${ver}?days=${days}`) : Promise.resolve(undefined)), [ver, days]);
  useEffect(() => { const id = setInterval(reload, 30000); return () => clearInterval(id); }, [reload]);

  const { labels, heat, steps } = useMemo(() => {
    const labels: Record<string, string> = {}, heat: Record<string, 0 | 1 | 2 | 3> = {};
    const steps = (st?.nodes ?? []).filter((n) => !/Gateway|Event$/.test(n.type) || n.active > 0);
    const max = Math.max(1, ...steps.map((n) => n.active));
    for (const n of steps) {
      const parts = [];
      if (n.active) parts.push(`${n.active} ${t("open")}`);
      if (n.avgHours) parts.push(hours(n.avgHours));
      if (n.overdue) parts.push(`${n.overdue} ${t("overdue")}`);
      if (n.incidents) parts.push(`${n.incidents} ${t("incidents")}`);
      labels[n.node] = parts.join(" · ");
      heat[n.node] = n.incidents || n.overdue ? 3 : n.active === 0 ? 0 : n.active / max > 0.66 ? 2 : 1;
    }
    return { labels, heat, steps: steps.sort((a, b) => b.active - a.active || b.avgHours - a.avgHours) };
  }, [st]);
  const s = steps.find((x) => x.node === sel);

  return (
    <Page title={t("Flow monitor")}>
      <div className="filters">
        <Dropdown options={(procs ?? []).map((p) => ({ value: p.key, label: p.name }))} value={proc} onChange={setProc} placeholder={t("Case type")} />
        <Dropdown options={(vers ?? []).map((v) => ({ value: v.id, label: `v${v.no} · ${t(v.status === "published" ? "Published" : v.status === "deprecated" ? "Deprecated" : v.status)}` }))} value={ver} onChange={setVer} placeholder={t("Version")} />
        <div className="seg">{[7, 30, 90].map((d) => <button key={d} className={days === d ? "on" : ""} onClick={() => setDays(d)}>{d} {t("days")}</button>)}</div>
      </div>
      <Err e={error} />
      {st && (
        <div className="cols">
          <section className="card">
            <div className="legend">
              <span><i style={{ background: "#fcf4d6" }} />{t("Some open cases")}</span>
              <span><i style={{ background: "#ffd8a8" }} />{t("Busiest steps")}</span>
              <span><i style={{ background: "#ffb3b8" }} />{t("Overdue or incident")}</span>
            </div>
            <MonitorViewer xml={st.bpmn} labels={labels} heat={heat} selected={sel} onSelect={setSel} />
          </section>
          <section className="card">
            {s ? (
              <>
                <h3>{s.name}</h3>
                <table className="tbl"><tbody>
                  <tr><td>{t("Open now")}</td><td className="r">{s.active}</td></tr>
                  <tr><td>{t("Overdue")}</td><td className={"r" + (s.overdue ? " late-t" : "")}>{s.overdue}</td></tr>
                  <tr><td>{t("Incidents")}</td><td className={"r" + (s.incidents ? " late-t" : "")}>{s.incidents}</td></tr>
                  <tr><td>{t("Completed in the period")}</td><td className="r">{s.passed}</td></tr>
                  <tr><td>{t("Average time in step")}</td><td className="r">{hours(s.avgHours)}</td></tr>
                  <tr><td>{t("9 in 10 within")}</td><td className="r">{hours(s.p90Hours)}</td></tr>
                </tbody></table>
                <div className="bar"><Link className="btn-s" href={`/ops/cases?version=${ver}&node=${s.node}`}>{t("Cases at this step")}</Link>
                  <button className="link" onClick={() => setSel(undefined)}>{t("All steps")}</button></div>
              </>
            ) : (
              <>
                <h3>{t("Steps")}</h3>
                <table className="tbl">
                  <thead><tr><th>{t("Step")}</th><th className="r">{t("Open")}</th><th className="r">{t("Average")}</th></tr></thead>
                  <tbody>{steps.filter((n) => n.active || n.passed).map((n) => (
                    <tr key={n.node} onClick={() => setSel(n.node)} style={{ cursor: "pointer" }}>
                      <td>{n.name}{(n.overdue || n.incidents) ? <span className="late-t small"> · {n.overdue + n.incidents}</span> : null}</td>
                      <td className="r">{n.active}</td><td className="r">{hours(n.avgHours)}</td>
                    </tr>
                  ))}</tbody>
                </table>
              </>
            )}
          </section>
        </div>
      )}
    </Page>
  );
}

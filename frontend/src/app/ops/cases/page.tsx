"use client";
// Case explorer: find cases by type, step, stage, person or SLA and open the
// inspector. Filters come from the URL so other views can link here.
import Link from "next/link";
import { useEffect, useState } from "react";
import { api, Case, ProcessDef } from "@/lib/api";
import { dt } from "@/lib/format";
import { t } from "@/lib/i18n";
import { useLoad } from "@/lib/useLoad";
import Dropdown from "@/components/Dropdown";
import { Empty, Err, Page, Status } from "@/components/ui";

type Row = Case & { steps?: string[] | null; stageName?: string; assignee: string; dueAt?: string; overdue: boolean; incident: boolean };
const KEYS = ["process", "version", "node", "stage", "status", "assignee", "sla", "q"] as const;

export default function Explorer() {
  const [f, setF] = useState<Record<string, string>>({ status: "running" });
  useEffect(() => {
    const q = new URLSearchParams(location.search);
    const init: Record<string, string> = { status: "running" };
    KEYS.forEach((k) => { const v = q.get(k); if (v) init[k] = v; });
    setF(init);
  }, []);
  const qs = new URLSearchParams(Object.entries(f).filter(([, v]) => v)).toString();
  const { data: procs } = useLoad(() => api.get<ProcessDef[]>("/api/processes"));
  const { data, error } = useLoad(() => api.get<Row[]>(`/api/monitor/cases?${qs}`), [qs]);
  const set = (k: string, v: string) => setF({ ...f, [k]: v });
  const extra = (["version", "node", "stage", "assignee"] as const).filter((k) => f[k]);
  return (
    <Page title={t("Cases")}>
      <div className="filters">
        <input placeholder={t("Case no. or title")} value={f.q ?? ""} onChange={(e) => set("q", e.target.value)} style={{ maxWidth: 240 }} />
        <Dropdown placeholder={t("All case types")} value={f.process} onChange={(v) => set("process", v)} options={(procs ?? []).map((p) => ({ value: p.key, label: p.name }))} />
        <Dropdown value={f.status} onChange={(v) => set("status", v)} placeholder={t("All statuses")}
          options={[{ value: "running", label: t("In progress") }, { value: "completed", label: t("Completed") }, { value: "cancelled", label: t("Cancelled") }, { value: "failed", label: t("Failed") }]} />
        <Dropdown value={f.sla} onChange={(v) => set("sla", v)} placeholder={t("Any SLA")} options={[{ value: "overdue", label: t("Overdue") }, { value: "incident", label: t("With an incident") }]} />
      </div>
      {extra.length > 0 && (
        <div className="filters">
          {extra.map((k) => <button key={k} className="btn-s" onClick={() => set(k, "")}>{t(k === "node" ? "Step" : k === "version" ? "Version" : k === "stage" ? "Stage" : "Assignee")}: {k === "version" ? "…" + f[k].slice(-6) : f[k]} ×</button>)}
        </div>
      )}
      <Err e={error} />
      {data?.length ? (
        <table className="tbl">
          <thead><tr><th>{t("Case")}</th><th>{t("Stage")}</th><th>{t("Waiting at")}</th><th>{t("Assignee")}</th><th>{t("Due")}</th><th>{t("Status")}</th></tr></thead>
          <tbody>{data.map((c) => (
            <tr key={c.id} className={c.overdue || c.incident ? "late" : ""}>
              <td><Link href={`/ops/cases/${c.id}`}>{c.number}</Link><div className="small muted">{c.title}</div></td>
              <td>{c.stageName || c.stage || "-"}</td><td>{(c.steps ?? []).join(", ") || "-"}{c.incident && <div className="small late-t">{t("Incident")}</div>}</td>
              <td>{c.assignee || "-"}</td><td className={"nw" + (c.overdue ? " late-t" : "")}>{dt(c.dueAt)}</td><td><Status s={c.status} /></td>
            </tr>
          ))}</tbody>
        </table>
      ) : data && <Empty>-</Empty>}
      {data && data.length >= 500 && <div className="small muted mt">{t("Showing the first 500 cases; narrow the filters")}</div>}
    </Page>
  );
}

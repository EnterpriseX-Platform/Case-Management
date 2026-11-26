"use client";
import { t } from "@/lib/i18n";
import Link from "next/link";
import { useState } from "react";
import { api, ProcessDef, Queue, UserProfile, Work } from "@/lib/api";
import { dt, loopLabel, overdue } from "@/lib/format";
import { useLoad } from "@/lib/useLoad";
import Dropdown from "@/components/Dropdown";
import { Err, Page, Status } from "@/components/ui";

type Dash = { casesByStatus: Record<string, number>; steps: { version: string; no: number; node: string; name: string; open: number; overdue: number }[]; workByRole: Record<string, number>; overdue: number; openWork: number };

export default function Queues() {
  const [proc, setProc] = useState("");
  const { data: procs } = useLoad(() => api.get<ProcessDef[]>("/api/processes"));
  const { data: d, error } = useLoad(() => api.get<Dash>(`/api/dashboard?process=${proc}`), [proc]);
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
    <Page title={t("Work queues")}>
      <div className="filters"><Dropdown placeholder={t("All case types")} value={proc} onChange={setProc} options={(procs ?? []).map((p) => ({ value: p.key, label: p.name }))} /></div>
      <Err e={error ?? e} />{msg && <div className="ok">{msg}</div>}
      <section className="card">
                <div className="filters">
          <Dropdown placeholder={t("All queues")} value={only} onChange={setOnly}
            options={[{ value: "late", label: t("Overdue only") }, ...Object.keys(d?.workByRole ?? {}).map((r) => ({ value: r, label: `${t("Queue")} ${r.replace(/^queue:/, "")}` }))]} />
        </div>
        {picked.length > 0 && (
          <div className="bulkbar">
            <span>{picked.length} {t("selected")}</span>
            <div style={{ minWidth: 200 }}><Dropdown options={(users ?? []).map((u) => ({ value: u.id, label: u.name || u.id }))} value={target} onChange={setTarget} placeholder={t("User or queue")} allowCustom /></div>
            <button className="btn-s" disabled={!target} onClick={() => bulk("reassign")}>{t("Assign")}</button>
            <div style={{ minWidth: 200 }}><Dropdown options={[...(queues ?? []).map((q) => ({ value: q.key, label: `${t("Queue")}: ${q.name}` })), ...roleOpts]} value={dest} onChange={setDest} placeholder={t("Transfer to")} /></div>
            <button className="btn-s" disabled={!dest} onClick={() => bulk("transfer")}>{t("Transfer")}</button>
            <button className="btn-s" onClick={() => bulk("release")}>{t("Return to queue")}</button>
            <button className="btn-s" onClick={() => bulk("urgency", 20)}>{t("Urgency")} +20</button>
            <button className="btn-s" onClick={() => setPicked([])}>{t("Clear")}</button>
          </div>
        )}
        <table className="tbl">
          <thead><tr>
            <th><button className={"check" + (picked.length && picked.length === list.length ? " on" : "")} onClick={() => setPicked(picked.length === list.length ? [] : list.map((w) => w.id))}>{picked.length && picked.length === list.length ? "✓" : ""}</button></th>
            <th className="r">{t("Urgency")}</th><th>{t("Case")}</th><th>{t("Step")}</th><th>{t("Role")}</th><th>{t("Assignee")}</th><th>{t("Due")}</th>
          </tr></thead>
          <tbody>{list.map((w) => (
            <tr key={w.id} className={overdue(w.dueAt) ? "late" : ""}>
              <td><button className={"check" + (picked.includes(w.id) ? " on" : "")} onClick={() => setPicked(picked.includes(w.id) ? picked.filter((x) => x !== w.id) : [...picked, w.id])}>{picked.includes(w.id) ? "✓" : ""}</button></td>
              <td className="r urg">{w.urgency ?? ""}</td>
              <td className="nw"><Link href={`/cases/${w.caseId}`}>{w.caseNumber}</Link></td>
              <td>{w.name}{loopLabel(w.input)}</td><td>{w.queue ? `${t("Queue")} ${w.queue}` : w.role}</td><td>{w.assignee || "-"}{w.delegatedFrom && <span className="small muted"> ({t("for")} {w.delegatedFrom})</span>}</td><td className="nw">{dt(w.dueAt)}</td>
            </tr>
          ))}</tbody>
        </table>
      </section>

    </Page>
  );
}

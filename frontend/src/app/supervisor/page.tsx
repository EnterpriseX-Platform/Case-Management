"use client";
import Link from "next/link";
import { useState } from "react";
import { api, ProcessDef, Work } from "@/lib/api";
import { dt, loopLabel, overdue } from "@/lib/format";
import { useLoad } from "@/lib/useLoad";
import Dropdown from "@/components/Dropdown";
import { Err, Page, Status } from "@/components/ui";

type Dash = { casesByStatus: Record<string, number>; steps: { version: string; no: number; node: string; name: string; open: number; overdue: number }[]; workByRole: Record<string, number>; overdue: number; openWork: number };

export default function Supervisor() {
  const [proc, setProc] = useState("");
  const { data: procs } = useLoad(() => api.get<ProcessDef[]>("/api/processes"));
  const { data: d, error } = useLoad(() => api.get<Dash>(`/api/dashboard?process=${proc}`), [proc]);
  const { data: work, reload } = useLoad(() => api.get<Work[]>("/api/work?all=1"));
  const [e, setE] = useState<string>();
  const [who, setWho] = useState<Record<string, string>>({});
  const [only, setOnly] = useState("");
  const list = (work ?? []).filter((w) => !only || (only === "late" ? overdue(w.dueAt) : w.role === only));
  return (
    <Page title="ภาพรวมและคิวงาน">
      <div className="filters"><Dropdown placeholder="ทุกประเภทเรื่อง" value={proc} onChange={setProc} options={(procs ?? []).map((p) => ({ value: p.key, label: p.name }))} /></div>
      <Err e={error ?? e} />
      {d && (
        <div className="row2">
          <section className="card">
            <h3>งานค้างตามขั้นตอน</h3>
            <table className="tbl">
              <thead><tr><th>ขั้นตอน</th><th>เวอร์ชัน</th><th className="r">ค้าง</th><th className="r">เกินกำหนด</th></tr></thead>
              <tbody>{d.steps.map((x) => <tr key={x.version + x.node}><td>{x.name}</td><td>v{x.no}</td><td className="r">{x.open}</td><td className={"r" + (x.overdue ? " late-t" : "")}>{x.overdue}</td></tr>)}</tbody>
            </table>
          </section>
          <section className="card">
            <h3>สรุป</h3>
            <table className="tbl"><tbody>
              {Object.entries(d.casesByStatus).map(([k, v]) => <tr key={k}><td><Status s={k} /></td><td className="r">{v} เรื่อง</td></tr>)}
              <tr><td>งานค้างทั้งหมด</td><td className="r">{d.openWork}</td></tr>
              <tr><td>งานเกินกำหนด</td><td className="r late-t">{d.overdue}</td></tr>
              {Object.entries(d.workByRole).map(([k, v]) => <tr key={k}><td className="muted">คิว {k}</td><td className="r">{v}</td></tr>)}
            </tbody></table>
          </section>
        </div>
      )}
      <section className="card mt">
        <h3>คิวงานทั้งหมด</h3>
        <div className="filters">
          <Dropdown placeholder="ทุกคิว" value={only} onChange={setOnly}
            options={[{ value: "late", label: "เฉพาะเกินกำหนด" }, ...Object.keys(d?.workByRole ?? {}).map((r) => ({ value: r, label: `คิว ${r}` }))]} />
        </div>
        <table className="tbl">
          <thead><tr><th>เรื่อง</th><th>ขั้นตอน</th><th>บทบาท</th><th>ผู้รับผิดชอบ</th><th>กำหนดเสร็จ</th><th>มอบหมายให้</th></tr></thead>
          <tbody>{list.map((w) => (
            <tr key={w.id} className={overdue(w.dueAt) ? "late" : ""}>
              <td className="nw"><Link href={`/cases/${w.caseId}`}>{w.caseNumber}</Link></td>
              <td>{w.name}{loopLabel(w.input)}</td><td>{w.role}</td><td>{w.assignee || "-"}</td><td className="nw">{dt(w.dueAt)}</td>
              <td className="nw">
                <input className="sm" placeholder="รหัสผู้ใช้" value={who[w.id] ?? ""} onChange={(x) => setWho({ ...who, [w.id]: x.target.value })} />
                <button className="btn-s" onClick={() => api.post(`/api/work/${w.id}/reassign`, { assignee: who[w.id] ?? "" }).then(reload).catch((x) => setE(x.message))}>{who[w.id] ? "มอบหมาย" : "คืนคิว"}</button>
              </td>
            </tr>
          ))}</tbody>
        </table>
      </section>
    </Page>
  );
}

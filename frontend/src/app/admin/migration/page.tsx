"use client";
import { useState } from "react";
import { api, ProcessDef, Version } from "@/lib/api";
import { useLoad } from "@/lib/useLoad";
import Dropdown from "@/components/Dropdown";
import { Err, Field, Page } from "@/components/ui";

type Res = { jobId?: string; dryRun: boolean; diff: { added: any[]; removed: any[]; changed: any[] }; unmapped: string[]; cases: { caseId: string; number: string; moves: Record<string, string>; error?: string }[]; OK: number; Failed: number };

export default function Migration() {
  const { data: procs } = useLoad(() => api.get<ProcessDef[]>("/api/processes"));
  const [key, setKey] = useState("");
  const { data: vers } = useLoad(() => (key ? api.get<Version[]>(`/api/processes/${key}/versions`) : Promise.resolve([])), [key]);
  const [from, setFrom] = useState(""), [to, setTo] = useState("");
  const [map, setMap] = useState<Record<string, string>>({});
  const [res, setRes] = useState<Res>();
  const [e, setE] = useState<string>();
  const vo = (vers ?? []).map((v) => ({ value: v.id, label: `v${v.no} · ${v.fiscalYear ? v.fiscalYear + 543 : "-"} · ${v.status}` }));
  const target = vers?.find((v) => v.id === to);
  const tnodes = target ? Array.from(target.bpmn.matchAll(/<(\w+)\s[^>]*id="([^"]+)"[^>]*name="([^"]*)"/g)).filter((m) => /Task|task|Event|subProcess|callActivity/.test(m[1])).map((m) => ({ value: m[2], label: m[3] || m[2], hint: m[1] })) : [];
  const run = (dryRun: boolean) => api.post<Res>("/api/migrations", { from, to, mapping: map, dryRun }).then((r) => { setRes(r); setE(undefined); }).catch((x) => setE(x.message));
  return (
    <Page title="ย้ายเรื่องข้ามเวอร์ชัน">
      <div className="card">
        <div className="row3">
          <Field label="ประเภทเรื่อง"><Dropdown options={(procs ?? []).map((p) => ({ value: p.key, label: p.name }))} value={key} onChange={(k) => { setKey(k); setFrom(""); setTo(""); setRes(undefined); }} /></Field>
          <Field label="จากเวอร์ชัน"><Dropdown options={vo} value={from} onChange={setFrom} /></Field>
          <Field label="ไปเวอร์ชัน"><Dropdown options={vo} value={to} onChange={setTo} /></Field>
        </div>
        <div className="bar"><button className="btn-o" disabled={!from || !to} onClick={() => run(true)}>ทดลองย้าย</button></div>
        <Err e={e} />
      </div>
      {res && (
        <section className="card mt">
          <h3>ผล{res.dryRun ? "การทดลอง" : "การย้าย"}</h3>
          <div>ย้ายได้ {res.OK} เรื่อง · ติดปัญหา {res.Failed} เรื่อง · ขั้นตอนเพิ่ม {res.diff.added.length} · ลบ {res.diff.removed.length} · แก้ไข {res.diff.changed.length}</div>
          {res.unmapped.length > 0 && (
            <>
              <h3 className="mt">ขั้นตอนที่ถูกลบแต่ยังมีเรื่องค้าง — เลือกขั้นตอนปลายทาง</h3>
              <table className="tbl"><tbody>{res.unmapped.map((n) => (
                <tr key={n}><td>{res.diff.removed.find((r) => r.id === n)?.name || n}</td>
                  <td><Dropdown options={tnodes} value={map[n]} onChange={(v) => setMap({ ...map, [n]: v })} /></td></tr>
              ))}</tbody></table>
            </>
          )}
          <table className="tbl mt">
            <thead><tr><th>เรื่อง</th><th>ขั้นตอนเดิม → ขั้นตอนใหม่</th><th>ผล</th></tr></thead>
            <tbody>{res.cases.map((c) => (
              <tr key={c.caseId}><td>{c.number}</td><td>{Object.entries(c.moves).map(([a, b]) => (a === b ? a : `${a} → ${b}`)).join(", ")}</td><td>{c.error ?? "ย้ายได้"}</td></tr>
            ))}</tbody>
          </table>
          {res.dryRun && <div className="bar"><button className="btn-o" onClick={() => run(true)}>ทดลองอีกครั้ง</button>
            <button className="btn" disabled={res.unmapped.some((n) => !map[n])} onClick={() => { if (confirm("ยืนยันย้ายเรื่องไปเวอร์ชันใหม่")) run(false); }}>ย้ายจริง</button></div>}
        </section>
      )}
    </Page>
  );
}

"use client";
import Link from "next/link";
import { useState } from "react";
import { api, ProcessDef } from "@/lib/api";
import { useLoad } from "@/lib/useLoad";
import Dropdown from "@/components/Dropdown";
import { Empty, Err, Field, Page } from "@/components/ui";

export default function Designer() {
  const { data, error, reload } = useLoad(() => api.get<ProcessDef[]>("/api/processes"));
  const [n, setN] = useState<Partial<ProcessDef> | null>(null);
  const [e, setE] = useState<string>();
  return (
    <Page title="ประเภทเรื่องและ flow" actions={<button className="btn" onClick={() => setN({})}>สร้างประเภทเรื่อง</button>}>
      <Err e={error ?? e} />
      {n && (
        <div className="card">
          <div className="row3">
            <Field label="รหัส (ภาษาอังกฤษตัวเล็ก)"><input value={n.key ?? ""} onChange={(x) => setN({ ...n, key: x.target.value.toLowerCase() })} /></Field>
            <Field label="ชื่อ"><input value={n.name ?? ""} onChange={(x) => setN({ ...n, name: x.target.value })} /></Field>
            <Field label="สืบทอดจาก"><Dropdown value={n.parentKey} onChange={(v) => setN({ ...n, parentKey: v })} options={(data ?? []).map((p) => ({ value: p.key, label: p.name }))} /></Field>
          </div>
          <div className="bar">
            <button className="btn" onClick={() => api.post("/api/processes", n).then(() => { setN(null); reload(); }).catch((x) => setE(x.message))}>บันทึก</button>
            <button className="link" onClick={() => setN(null)}>ยกเลิก</button>
          </div>
        </div>
      )}
      {data?.length ? (
        <table className="tbl">
          <thead><tr><th>รหัส</th><th>ชื่อ</th><th>หมวด</th><th>สืบทอดจาก</th></tr></thead>
          <tbody>
            {data.map((p) => (
              <tr key={p.key}><td><Link href={`/designer/${p.key}`}>{p.key}</Link></td><td>{p.name}</td><td>{p.category ?? "-"}</td><td>{p.parentKey ?? "-"}</td></tr>
            ))}
          </tbody>
        </table>
      ) : data && <Empty>ยังไม่มีประเภทเรื่อง</Empty>}
    </Page>
  );
}

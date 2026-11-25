"use client";
// Low-code screen builder: rows of columns, each column holds one widget.
import { useParams } from "next/navigation";
import { useEffect, useState } from "react";
import { api, Screen, Widget } from "@/lib/api";
import Dropdown from "@/components/Dropdown";
import { WIDGETS } from "@/components/ScreenRenderer";
import { Err, Field, Page } from "@/components/ui";

const KINDS = [
  { value: "task", label: "หน้าทำงานของขั้นตอน" }, { value: "case", label: "หน้าภาพรวมเรื่อง" },
  { value: "portal", label: "หน้าผู้ยื่นเรื่อง" }, { value: "dashboard", label: "หน้าติดตามงาน" },
];
const W = [12, 8, 7, 6, 5, 4, 3].map((w) => ({ value: String(w), label: `${w}/12` }));

export default function ScreenBuilder() {
  const { key } = useParams<{ key: string }>();
  const [s, setS] = useState<Screen>({ key, name: "", kind: "task", layout: { rows: [{ cols: [{ width: 12, widget: "form" }] }] } });
  const [e, setE] = useState<string>();
  const [msg, setMsg] = useState<string>();
  useEffect(() => { api.get<Screen>(`/api/screens/${key}`).then(setS).catch(() => {}); }, [key]);
  const rows = s.layout.rows;
  const setRows = (r: typeof rows) => setS({ ...s, layout: { rows: r } });
  const setCol = (i: number, j: number, p: Partial<Widget>) => setRows(rows.map((r, a) => a !== i ? r : { cols: r.cols.map((c, b) => (b === j ? { ...c, ...p } : c)) }));
  const used = (r: (typeof rows)[0]) => r.cols.reduce((n, c) => n + (c.width || 12), 0);
  return (
    <Page title={`หน้าจอ · ${key}`} actions={<button className="btn" onClick={() => api.put<Screen>(`/api/screens/${key}`, s).then((x) => { setS(x); setMsg("บันทึกแล้ว"); setE(undefined); }).catch((x) => { setE(x.message); setMsg(undefined); })}>บันทึก</button>}>
      <Err e={e} />{msg && <div className="ok">{msg}</div>}
      <div className="row3">
        <Field label="ชื่อหน้าจอ"><input value={s.name} onChange={(x) => setS({ ...s, name: x.target.value })} /></Field>
        <Field label="ใช้สำหรับ"><Dropdown options={KINDS} value={s.kind} onChange={(k) => setS({ ...s, kind: k })} /></Field>
        <Field label="บทบาทที่ใช้หน้านี้"><Dropdown multiple allowCustom options={(s.roles ?? []).map((r) => ({ value: r, label: r }))} value={s.roles ?? []} onChange={(r) => setS({ ...s, roles: r })} placeholder="ทุกบทบาท" /></Field>
      </div>
      {rows.map((r, i) => (
        <div key={i} className="lrow">
          <div className="lrow-h">
            <span className="small muted">แถว {i + 1} · ใช้ {used(r)}/12</span>
            <button className="link" disabled={used(r) >= 12} onClick={() => setRows(rows.map((x, a) => a === i ? { cols: [...x.cols, { width: Math.max(12 - used(r), 3), widget: "markdown" }] } : x))}>เพิ่มคอลัมน์</button>
            <button className="link" disabled={i === 0} onClick={() => { const n = rows.slice(); [n[i - 1], n[i]] = [n[i], n[i - 1]]; setRows(n); }}>ขึ้น</button>
            <button className="link danger" onClick={() => setRows(rows.filter((_, a) => a !== i))}>ลบแถว</button>
          </div>
          <div className="lgrid">
            {r.cols.map((c, j) => (
              <div key={j} className="lcol" style={{ gridColumn: `span ${c.width || 12}` }}>
                <Dropdown options={WIDGETS} value={c.widget} onChange={(w) => setCol(i, j, { widget: w })} />
                <Dropdown options={W} value={String(c.width)} onChange={(w) => setCol(i, j, { width: Number(w) })} />
                {c.widget === "iframe" && <input placeholder="https://" value={c.url ?? ""} onChange={(x) => setCol(i, j, { url: x.target.value })} />}
                {c.widget === "markdown" && <textarea rows={2} value={c.text ?? ""} onChange={(x) => setCol(i, j, { text: x.target.value })} />}
                <button className="link danger" onClick={() => setRows(rows.map((x, a) => a === i ? { cols: x.cols.filter((_, b) => b !== j) } : x))}>ลบ</button>
              </div>
            ))}
          </div>
        </div>
      ))}
      <button className="btn-o" onClick={() => setRows([...rows, { cols: [{ width: 12, widget: "history" }] }])}>เพิ่มแถว</button>
    </Page>
  );
}

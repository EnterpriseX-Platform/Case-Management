"use client";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useRef, useState } from "react";
import { api, Version } from "@/lib/api";
import { useLoad } from "@/lib/useLoad";
import { Modeler, ModelerHandle } from "@/components/Bpmn";
import { Err, Field, Page, Status } from "@/components/ui";

export default function FlowEditor() {
  const { id } = useParams<{ id: string }>();
  const { data: v, error, setData } = useLoad(() => api.get<Version>(`/api/versions/${id}`), [id]);
  const m = useRef<ModelerHandle>(null);
  const [e, setE] = useState<string>();
  const [msg, setMsg] = useState<string>();
  const [warn, setWarn] = useState<string[]>([]);
  const [sel, setSel] = useState<{ id?: string; name?: string; type?: string }>({});
  const [fy, setFy] = useState("");
  if (!v) return <Page title="flow"><Err e={error} /></Page>;
  const draft = v.status === "draft";

  const save = async () => {
    setE(undefined); setMsg(undefined);
    try {
      const xml = await m.current!.getXML();
      const r = await api.put<{ version: Version; warnings: string[] }>(`/api/versions/${id}`, { bpmn: xml, fiscalYear: fy ? Number(fy) - 543 : 0 });
      setWarn(r.warnings ?? []);
      setMsg("บันทึกแล้ว");
      setData(r.version);
    } catch (x: any) { setE(x.message); }
  };
  const download = async () => {
    const xml = await m.current!.getXML();
    const a = document.createElement("a");
    a.href = URL.createObjectURL(new Blob([xml], { type: "application/xml" }));
    a.download = `${v.processKey}-v${v.no}.bpmn`;
    a.click();
  };

  return (
    <Page title={`${v.processKey} v${v.no}`} actions={<>
      <Status s={v.status} />
      <Link className="btn-o" href={`/designer/${v.processKey}`}>เวอร์ชันทั้งหมด</Link>
      <Link className="btn-o" href={`/designer/versions/${id}/mapping`}>หน้าจอของขั้นตอน</Link>
      <button className="btn-o" onClick={download}>ส่งออก BPMN</button>
      {draft && <button className="btn" onClick={save}>บันทึก</button>}
    </>}>
      <Err e={e} />
      {msg && <div className="ok">{msg}</div>}
      {warn.map((w) => <div key={w} className="warn">{w}</div>)}
      {!draft && <div className="warn">เวอร์ชันนี้{v.status === "published" ? "ใช้งานอยู่" : "ไม่ใช่ร่าง"} แก้ไขไม่ได้ ให้คัดลอกเป็นเวอร์ชันใหม่</div>}
      {draft && (
        <div className="row3">
          <Field label="ปีงบประมาณ (พ.ศ.)"><input inputMode="numeric" placeholder={v.fiscalYear ? String(v.fiscalYear + 543) : ""} value={fy} onChange={(x) => setFy(x.target.value)} /></Field>
          <Field label="องค์ประกอบที่เลือก"><div className="display">{sel.id ? `${sel.name || "(ไม่มีชื่อ)"} · ${sel.type} · ${sel.id}` : "-"}</div></Field>
        </div>
      )}
      <Modeler ref={m} xml={v.bpmn} readOnly={!draft} onSelect={(id, name, type) => setSel({ id, name, type })} />
    </Page>
  );
}

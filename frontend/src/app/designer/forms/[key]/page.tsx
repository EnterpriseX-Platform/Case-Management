"use client";
// Low-code form builder: palette -> field list (reorder) -> properties, with
// a live preview rendered by the same component used on task screens.
import { useParams } from "next/navigation";
import { useEffect, useState } from "react";
import { api, Field as F, Form } from "@/lib/api";
import Dropdown from "@/components/Dropdown";
import FormRenderer from "@/components/FormRenderer";
import { Err, Field, Page } from "@/components/ui";

const TYPES: { value: string; label: string }[] = [
  { value: "text", label: "ข้อความ" }, { value: "textarea", label: "ข้อความยาว" }, { value: "number", label: "ตัวเลข" },
  { value: "money", label: "จำนวนเงิน" }, { value: "date", label: "วันที่" }, { value: "select", label: "ตัวเลือก (เลือกหนึ่ง)" },
  { value: "multiselect", label: "ตัวเลือก (หลายรายการ)" }, { value: "radio", label: "ปุ่มเลือก" }, { value: "checkbox", label: "ช่องทำเครื่องหมาย" },
  { value: "file", label: "ไฟล์แนบ" }, { value: "table", label: "ตาราง" }, { value: "user", label: "ผู้ใช้" }, { value: "org", label: "หน่วยงาน" },
  { value: "display", label: "แสดงค่า (อ่านอย่างเดียว)" },
];
const WIDTH = [12, 9, 8, 6, 4, 3].map((w) => ({ value: String(w), label: `${w}/12` }));

export default function FormBuilder() {
  const { key } = useParams<{ key: string }>();
  const [f, setF] = useState<Form>({ key, name: "", schema: { fields: [] } });
  const [sel, setSel] = useState(-1);
  const [preview, setPreview] = useState<Record<string, any>>({});
  const [e, setE] = useState<string>();
  const [msg, setMsg] = useState<string>();
  useEffect(() => { api.get<Form>(`/api/forms/${key}`).then(setF).catch(() => {}); }, [key]);
  const fields = f.schema.fields;
  const setFields = (fs: F[]) => setF({ ...f, schema: { ...f.schema, fields: fs } });
  const upd = (p: Partial<F>) => setFields(fields.map((x, i) => (i === sel ? { ...x, ...p } : x)));
  const add = (type: string) => {
    let n = fields.length + 1;
    while (fields.some((x) => x.key === `field${n}`)) n++;
    setFields([...fields, { key: `field${n}`, label: TYPES.find((t) => t.value === type)!.label, type, width: 12 }]);
    setSel(fields.length);
  };
  const move = (d: number) => {
    const j = sel + d;
    if (j < 0 || j >= fields.length) return;
    const n = fields.slice(); [n[sel], n[j]] = [n[j], n[sel]]; setFields(n); setSel(j);
  };
  const cur = fields[sel];
  const save = () => api.put<Form>(`/api/forms/${key}`, f).then((x) => { setF(x); setMsg(`บันทึกแล้ว (ฉบับที่ ${x.version})`); setE(undefined); }).catch((x) => { setE(x.message); setMsg(undefined); });

  return (
    <Page title={`ฟอร์ม · ${key}`} actions={<button className="btn" onClick={save}>บันทึก</button>}>
      <Err e={e} />{msg && <div className="ok">{msg}</div>}
      <Field label="ชื่อฟอร์ม"><input value={f.name} onChange={(x) => setF({ ...f, name: x.target.value })} /></Field>
      <div className="builder">
        <div className="palette">
          <div className="side-h">เพิ่มช่อง</div>
          {TYPES.map((t) => <button key={t.value} className="pal" onClick={() => add(t.value)}>{t.label}</button>)}
        </div>
        <div className="canvas">
          <div className="side-h">ช่องในฟอร์ม</div>
          {fields.map((x, i) => (
            <div key={i} className={"fitem" + (i === sel ? " on" : "")} onClick={() => setSel(i)}>
              <span>{x.label}</span><span className="muted small">{x.key} · {TYPES.find((t) => t.value === x.type)?.label}{x.required ? " · บังคับ" : ""}</span>
            </div>
          ))}
          {cur && (
            <div className="bar">
              <button className="btn-s" onClick={() => move(-1)}>ขึ้น</button>
              <button className="btn-s" onClick={() => move(1)}>ลง</button>
              <button className="btn-s danger" onClick={() => { setFields(fields.filter((_, i) => i !== sel)); setSel(-1); }}>ลบ</button>
            </div>
          )}
        </div>
        <div className="props">
          <div className="side-h">คุณสมบัติ</div>
          {cur ? (
            <>
              <Field label="ชื่อที่แสดง"><input value={cur.label} onChange={(x) => upd({ label: x.target.value })} /></Field>
              <Field label="รหัสตัวแปร" hint="ใช้ในเงื่อนไขของ flow"><input value={cur.key} onChange={(x) => upd({ key: x.target.value.replace(/[^\w]/g, "") })} /></Field>
              <Field label="ชนิด"><Dropdown options={TYPES} value={cur.type} onChange={(t) => upd({ type: t })} /></Field>
              <Field label="ความกว้าง"><Dropdown options={WIDTH} value={String(cur.width ?? 12)} onChange={(w) => upd({ width: Number(w) })} /></Field>
              <div className="fld"><span className="fld-l">บังคับกรอก</span>
                <button type="button" className={"check" + (cur.required ? " on" : "")} onClick={() => upd({ required: !cur.required })}>{cur.required ? "✓" : ""}</button></div>
              {(cur.type === "number" || cur.type === "money") && (
                <div className="row2">
                  <Field label="ค่าต่ำสุด"><input value={cur.min ?? ""} onChange={(x) => upd({ min: x.target.value === "" ? undefined : Number(x.target.value) })} /></Field>
                  <Field label="ค่าสูงสุด"><input value={cur.max ?? ""} onChange={(x) => upd({ max: x.target.value === "" ? undefined : Number(x.target.value) })} /></Field>
                </div>
              )}
              {["select", "multiselect", "radio"].includes(cur.type) && (
                <Field label="ตัวเลือก" hint="หนึ่งบรรทัดต่อหนึ่งตัวเลือก รูปแบบ ค่า=ชื่อที่แสดง">
                  <textarea rows={5} value={(cur.options ?? []).map((o) => `${o.value}=${o.label}`).join("\n")}
                    onChange={(x) => upd({ options: x.target.value.split("\n").filter(Boolean).map((l) => { const [v, ...r] = l.split("="); return { value: v.trim(), label: (r.join("=") || v).trim() }; }) })} />
                </Field>
              )}
              {cur.type === "table" && (
                <Field label="คอลัมน์" hint="หนึ่งบรรทัดต่อคอลัมน์ รูปแบบ รหัส=ชื่อ:ชนิด">
                  <textarea rows={4} value={(cur.columns ?? []).map((c) => `${c.key}=${c.label}:${c.type}`).join("\n")}
                    onChange={(x) => upd({ columns: x.target.value.split("\n").filter(Boolean).map((l) => { const [k, r = ""] = l.split("="); const [lb, t = "text"] = r.split(":"); return { key: k.trim(), label: (lb || k).trim(), type: t.trim() }; }) })} />
                </Field>
              )}
              <Field label="แสดงเมื่อ" hint="เช่น proposedAmount < requestAmount"><input value={cur.visibleIf ?? ""} onChange={(x) => upd({ visibleIf: x.target.value || undefined })} /></Field>
              <Field label="คำอธิบายใต้ช่อง"><input value={cur.help ?? ""} onChange={(x) => upd({ help: x.target.value || undefined })} /></Field>
            </>
          ) : <div className="muted">-</div>}
        </div>
      </div>
      <section className="card mt">
        <h3>ตัวอย่าง</h3>
        <FormRenderer fields={fields} data={preview} onChange={setPreview} />
      </section>
    </Page>
  );
}

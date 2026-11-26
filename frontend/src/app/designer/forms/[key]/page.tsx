"use client";
// Low-code form builder: palette -> field list (reorder) -> properties, with
// a live preview rendered by the same component used on task screens.
import { t } from "@/lib/i18n";
import { useParams } from "next/navigation";
import { useEffect, useState } from "react";
import { api, Field as F, Form } from "@/lib/api";
import Dropdown from "@/components/Dropdown";
import FormRenderer from "@/components/FormRenderer";
import { Err, Field, Page } from "@/components/ui";

const TYPES: { value: string; label: string }[] = [
  { value: "text", label: "Text" }, { value: "textarea", label: "Long text" }, { value: "number", label: "Number" },
  { value: "money", label: "Amount" }, { value: "date", label: "Date" }, { value: "select", label: t("Select") },
  { value: "multiselect", label: "Multi-select" }, { value: "radio", label: "Radio" }, { value: "checkbox", label: "Checkbox" },
  { value: "file", label: "File upload" }, { value: "table", label: "Table" }, { value: "user", label: "User" }, { value: "org", label: "Organisation" },
  { value: "display", label: "Display (read-only)" },
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
  const save = () => api.put<Form>(`/api/forms/${key}`, f).then((x) => { setF(x); setMsg(`Saved (revision ${x.version})`); setE(undefined); }).catch((x) => { setE(x.message); setMsg(undefined); });

  return (
    <Page title={`Forms · ${key}`} actions={<button className="btn" onClick={save}>{t("Save")}</button>}>
      <Err e={e} />{msg && <div className="ok">{msg}</div>}
      <Field label={t("Form name")}><input value={f.name} onChange={(x) => setF({ ...f, name: x.target.value })} /></Field>
      <div className="builder">
        <div className="palette">
          <div className="side-h">{t("Add field")}</div>
          {TYPES.map((t) => <button key={t.value} className="pal" onClick={() => add(t.value)}>{t.label}</button>)}
        </div>
        <div className="canvas">
          <div className="side-h">{t("Form fields")}</div>
          {fields.map((x, i) => (
            <div key={i} className={"fitem" + (i === sel ? " on" : "")} onClick={() => setSel(i)}>
              <span>{x.label}</span><span className="muted small">{x.key} · {TYPES.find((t) => t.value === x.type)?.label}{x.required ? " · required" : ""}</span>
            </div>
          ))}
          {cur && (
            <div className="bar">
              <button className="btn-s" onClick={() => move(-1)}>{t("Up")}</button>
              <button className="btn-s" onClick={() => move(1)}>{t("Down")}</button>
              <button className="btn-s danger" onClick={() => { setFields(fields.filter((_, i) => i !== sel)); setSel(-1); }}>{t("Remove")}</button>
            </div>
          )}
        </div>
        <div className="props">
          <div className="side-h">{t("Properties")}</div>
          {cur ? (
            <>
              <Field label={t("Label")}><input value={cur.label} onChange={(x) => upd({ label: x.target.value })} /></Field>
              <Field label={t("Variable name")} hint="Used in flow conditions"><input value={cur.key} onChange={(x) => upd({ key: x.target.value.replace(/[^\w]/g, "") })} /></Field>
              <Field label={t("Type")}><Dropdown options={TYPES} value={cur.type} onChange={(t) => upd({ type: t })} /></Field>
              <Field label={t("Width")}><Dropdown options={WIDTH} value={String(cur.width ?? 12)} onChange={(w) => upd({ width: Number(w) })} /></Field>
              <div className="fld"><span className="fld-l">{t("Required")}</span>
                <button type="button" className={"check" + (cur.required ? " on" : "")} onClick={() => upd({ required: !cur.required })}>{cur.required ? "✓" : ""}</button></div>
              {(cur.type === "number" || cur.type === "money") && (
                <div className="row2">
                  <Field label={t("Min")}><input value={cur.min ?? ""} onChange={(x) => upd({ min: x.target.value === "" ? undefined : Number(x.target.value) })} /></Field>
                  <Field label={t("Max")}><input value={cur.max ?? ""} onChange={(x) => upd({ max: x.target.value === "" ? undefined : Number(x.target.value) })} /></Field>
                </div>
              )}
              {["select", "multiselect", "radio"].includes(cur.type) && (
                <Field label={t("Options")} hint="One option per line: value=label">
                  <textarea rows={5} value={(cur.options ?? []).map((o) => `${o.value}=${o.label}`).join("\n")}
                    onChange={(x) => upd({ options: x.target.value.split("\n").filter(Boolean).map((l) => { const [v, ...r] = l.split("="); return { value: v.trim(), label: (r.join("=") || v).trim() }; }) })} />
                </Field>
              )}
              {cur.type === "table" && (
                <Field label={t("Columns")} hint="One column per line: key=label:type">
                  <textarea rows={4} value={(cur.columns ?? []).map((c) => `${c.key}=${c.label}:${c.type}`).join("\n")}
                    onChange={(x) => upd({ columns: x.target.value.split("\n").filter(Boolean).map((l) => { const [k, r = ""] = l.split("="); const [lb, t = "text"] = r.split(":"); return { key: k.trim(), label: (lb || k).trim(), type: t.trim() }; }) })} />
                </Field>
              )}
              <Field label={t("Visible when")} hint="e.g. proposedAmount < requestAmount"><input value={cur.visibleIf ?? ""} onChange={(x) => upd({ visibleIf: x.target.value || undefined })} /></Field>
              <Field label={t("Helper text")}><input value={cur.help ?? ""} onChange={(x) => upd({ help: x.target.value || undefined })} /></Field>
            </>
          ) : <div className="muted">-</div>}
        </div>
      </div>
      <section className="card mt">
        <h3>{t("Preview")}</h3>
        <FormRenderer fields={fields} data={preview} onChange={setPreview} />
      </section>
    </Page>
  );
}

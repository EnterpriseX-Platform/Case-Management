"use client";
// Renders a low-code form schema. Used by task screens and by the form
// builder preview.
import Dropdown from "./Dropdown";
import DropZone from "./DropZone";
import { Field as F } from "@/lib/api";
import { money, visible } from "@/lib/format";

type Props = {
  fields: F[];
  data: Record<string, any>;
  onChange: (d: Record<string, any>) => void;
  readOnly?: boolean;
  errors?: Record<string, string>;
};

export default function FormRenderer({ fields, data, onChange, readOnly, errors = {} }: Props) {
  const set = (k: string, v: any) => onChange({ ...data, [k]: v });
  return (
    <div className="form-grid">
      {fields.filter((f) => visible(f.visibleIf, data)).map((f) => (
        <div key={f.key} className="fcell" style={{ gridColumn: `span ${f.width || 12}` }}>
          <div className="fld-l">{f.label}{f.required && f.type !== "display" && <span className="req"> *</span>}</div>
          <Input f={f} v={data[f.key]} set={(v) => set(f.key, v)} ro={readOnly || f.readOnly} />
          {f.help && <div className="muted small">{f.help}</div>}
          {errors[f.key] && <div className="ferr">{errors[f.key]}</div>}
        </div>
      ))}
    </div>
  );
}

function Input({ f, v, set, ro }: { f: F; v: any; set: (v: any) => void; ro?: boolean }) {
  const opts = (f.options ?? []).map((o) => ({ value: String(o.value), label: o.label }));
  switch (f.type) {
    case "display":
      return <div className="display">{typeof v === "number" ? money(v) : String(v ?? "-")}</div>;
    case "textarea":
      return <textarea rows={3} value={v ?? ""} disabled={ro} onChange={(e) => set(e.target.value)} />;
    case "number":
    case "money":
      return <input inputMode="decimal" disabled={ro} className={f.type === "money" ? "num" : ""}
        value={v ?? ""} onChange={(e) => {
          const s = e.target.value.replace(/,/g, "");
          set(s === "" ? undefined : isNaN(Number(s)) ? s : Number(s));
        }} />;
    case "date":
      return <input type="date" disabled={ro} value={v ?? ""} onChange={(e) => set(e.target.value)} />;
    case "select":
    case "user":
    case "org":
      return <Dropdown options={opts} value={v} onChange={set} disabled={ro} allowCustom={f.type !== "select"} />;
    case "multiselect":
      return <Dropdown multiple options={opts} value={v ?? []} onChange={set} disabled={ro} allowCustom />;
    case "radio":
      return (
        <div className="radios">
          {opts.map((o) => (
            <button type="button" key={o.value} disabled={ro}
              className={"radio" + (String(v) === o.value ? " on" : "")} onClick={() => set(o.value)}>{o.label}</button>
          ))}
        </div>
      );
    case "checkbox":
      return (
        <button type="button" disabled={ro} className={"check" + (v ? " on" : "")} onClick={() => set(!v)}>
          {v ? "✓" : ""}
        </button>
      );
    case "file":
      return ro ? <div className="display">{Array.isArray(v) ? v.join(", ") : v ?? "-"}</div>
        : <DropZone multiple onFiles={(fs) => set(fs.map((x) => x.name))} />;
    case "table": {
      const rows: Record<string, any>[] = Array.isArray(v) ? v : [];
      const cols = f.columns ?? [];
      return (
        <div>
          <table className="tbl tbl-in">
            <thead><tr>{cols.map((c) => <th key={c.key}>{c.label}</th>)}{!ro && <th />}</tr></thead>
            <tbody>
              {rows.map((r, i) => (
                <tr key={i}>
                  {cols.map((c) => (
                    <td key={c.key}>
                      <Input f={c} v={r[c.key]} ro={ro} set={(x) => {
                        const n = rows.slice(); n[i] = { ...r, [c.key]: x }; set(n);
                      }} />
                    </td>
                  ))}
                  {!ro && <td><button type="button" className="link" onClick={() => set(rows.filter((_, j) => j !== i))}>ลบ</button></td>}
                </tr>
              ))}
            </tbody>
          </table>
          {!ro && <button type="button" className="btn-s" onClick={() => set([...rows, {}])}>เพิ่มแถว</button>}
        </div>
      );
    }
    default:
      return <input disabled={ro} value={v ?? ""} onChange={(e) => set(e.target.value)} />;
  }
}

/** Client-side mirror of the server validation for fast feedback. */
export function validate(fields: F[], data: Record<string, any>) {
  const e: Record<string, string> = {};
  for (const f of fields) {
    if (!visible(f.visibleIf, data) || f.type === "display") continue;
    const v = data[f.key];
    const empty = v === undefined || v === null || v === "" || (Array.isArray(v) && !v.length);
    if (f.required && empty) e[f.key] = "จำเป็นต้องกรอก";
    else if (!empty && (f.type === "number" || f.type === "money")) {
      if (typeof v !== "number") e[f.key] = "ต้องเป็นตัวเลข";
      else if (f.min !== undefined && v < f.min) e[f.key] = `ต้องไม่น้อยกว่า ${money(f.min)}`;
      else if (f.max !== undefined && v > f.max) e[f.key] = `ต้องไม่เกิน ${money(f.max)}`;
    }
  }
  return e;
}

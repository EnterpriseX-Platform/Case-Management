"use client";
// Renders a low-code form schema. Used by task screens and by the form
// builder preview.
import { t } from "@/lib/i18n";
import Dropdown from "./Dropdown";
import DropZone from "./DropZone";
import { Field as F, FormPage } from "@/lib/api";
import { money, visible } from "@/lib/format";
import { evalCond } from "@/lib/cond";
import { useEffect, useState } from "react";

type Props = {
  fields: F[];
  data: Record<string, any>;
  onChange: (d: Record<string, any>) => void;
  readOnly?: boolean;
  errors?: Record<string, string>;
  pages?: FormPage[];
};

/** Whether a field is shown for the current data (no-code rule, or expression). */
export const isShown = (f: F, data: Record<string, any>) => (f.show ? evalCond(f.show, data) : visible(f.visibleIf, data));
const isLocked = (f: F, data: Record<string, any>) => !!f.readOnly || (!!f.lock?.rules?.length && evalCond(f.lock, data));
/** Pages that apply to this data (a page can be skipped by its condition). */
export const activePages = (pages: FormPage[] | undefined, data: Record<string, any>) =>
  (pages ?? []).filter((p) => !(p.skip?.rules?.length && evalCond(p.skip, data)));

/** Initial values from field defaults ("today" for dates). */
export function withDefaults(fields: F[], data: Record<string, any>) {
  const out = { ...data };
  for (const f of fields) {
    if (out[f.key] !== undefined || f.default === undefined || f.default === "") continue;
    out[f.key] = f.default === "today" ? new Date().toISOString().slice(0, 10)
      : f.type === "number" || f.type === "money" ? Number(f.default) : f.type === "checkbox" ? f.default === "true" : f.default;
  }
  return out;
}

export default function FormRenderer({ fields, data, onChange, readOnly, errors = {}, pages }: Props) {
  const set = (k: string, v: any) => onChange({ ...data, [k]: v });
  // apply defaults once for empty values
  useEffect(() => {
    const d = withDefaults(fields, data);
    if (!readOnly && Object.keys(d).some((k) => d[k] !== data[k])) onChange(d);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fields]);
  const act = activePages(pages, data);
  const [pi, setPi] = useState(0);
  const page = act.length > 1 ? act[Math.min(pi, act.length - 1)] : undefined;
  // jump to the first page that has an error
  useEffect(() => {
    if (!act.length) return;
    const bad = fields.find((f) => errors[f.key]);
    if (bad?.page) { const j = act.findIndex((p) => p.key === bad.page); if (j >= 0) setPi(j); }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [errors]);
  const onPage = (f: F) => !page || (f.page ?? act[0]?.key) === page.key;
  return (
    <div className="form-wrap">
      {page && (
        <ol className="fsteps">
          {act.map((p, j) => <li key={p.key} className={j === pi ? "on" : j < pi ? "done" : ""}><button type="button" onClick={() => setPi(j)}>{p.title}</button></li>)}
        </ol>
      )}
      <div className="form-grid">
        {fields.filter((f) => onPage(f) && isShown(f, data)).map((f) => f.type === "section" ? (
          <h4 key={f.key} className="fsection" style={{ gridColumn: "span 12" }}>{f.label}</h4>
        ) : (
          <div key={f.key} className="fcell" style={{ gridColumn: `span ${f.width || 12}`, ["--sm" as any]: `span ${f.widthSm || 12}` }}>
            <div className="fld-l">{f.label}{f.required && f.type !== "display" && <span className="req"> *</span>}</div>
            <Input f={f} v={data[f.key]} set={(v) => set(f.key, v)} ro={readOnly || isLocked(f, data)} />
            {f.help && <div className="muted small">{f.help}</div>}
            {errors[f.key] && <div className="ferr">{errors[f.key]}</div>}
          </div>
        ))}
      </div>
      {page && (
        <div className="fpage-nav">
          <button type="button" className="btn-o" disabled={pi === 0} onClick={() => setPi(pi - 1)}>{t("Back")}</button>
          {pi < act.length - 1 && <button type="button" className="btn-o" onClick={() => setPi(pi + 1)}>{t("Next")}</button>}
        </div>
      )}
    </div>
  );
}

export function Input({ f, v, set, ro }: { f: F; v: any; set: (v: any) => void; ro?: boolean }) {
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
                  {!ro && <td><button type="button" className="link" onClick={() => set(rows.filter((_, j) => j !== i))}>{t("Remove")}</button></td>}
                </tr>
              ))}
            </tbody>
          </table>
          {!ro && <button type="button" className="btn-s" onClick={() => set([...rows, {}])}>{t("Add row")}</button>}
        </div>
      );
    }
    default:
      return <input disabled={ro} value={v ?? ""} onChange={(e) => set(e.target.value)} />;
  }
}

/** Client-side mirror of the server validation for fast feedback. */
export function validate(fields: F[], data: Record<string, any>, pages?: FormPage[]) {
  const e: Record<string, string> = {};
  const skipped = new Set((pages ?? []).filter((p) => !activePages([p], data).length).map((p) => p.key));
  for (const f of fields) {
    if ((f.page && skipped.has(f.page)) || !isShown(f, data) || f.type === "display" || f.type === "section") continue;
    const v = data[f.key];
    const empty = v === undefined || v === null || v === "" || (Array.isArray(v) && !v.length);
    const say = (def: string) => (f.message ? f.message : def);
    if (f.required && empty) e[f.key] = t("Required");
    else if (!empty && (f.type === "number" || f.type === "money")) {
      if (typeof v !== "number") e[f.key] = "Must be a number";
      else if (f.min !== undefined && v < f.min) e[f.key] = say(`Must be at least ${money(f.min)}`);
      else if (f.max !== undefined && v > f.max) e[f.key] = say(`Must be at most ${money(f.max)}`);
    } else if (!empty && typeof v === "string") {
      if (f.minLen && [...v].length < f.minLen) e[f.key] = say(`At least ${f.minLen} characters`);
      else if (f.maxLen && [...v].length > f.maxLen) e[f.key] = say(`At most ${f.maxLen} characters`);
      else if (f.pattern) { try { if (!new RegExp(`^(?:${f.pattern})$`).test(v)) e[f.key] = say("Wrong format"); } catch { /* bad pattern: server decides */ } }
    }
  }
  return e;
}

"use client";
// Low-code form / page builder: components and case data on the left, the
// form as people will see it in the middle (any device width), and
// properties, no-code conditions and validation on the right. Forms can have
// several pages; a page can be skipped by a condition, which is how pages
// link and branch. The same schema renders task screens and exports to code.
import { t } from "@/lib/i18n";
import { useParams } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { api, CaseModel, Cond, CondRule, DataFieldDef, Field as F, Form, FormPage, ProcessDef, Version } from "@/lib/api";
import Dropdown from "@/components/Dropdown";
import FormRenderer, { Input, validate } from "@/components/FormRenderer";
import { compileCond, OPS } from "@/lib/cond";
import { Err } from "@/components/ui";

type Kind = { value: string; label: string; group: "input" | "layout" | "show" };
const KINDS: Kind[] = [
  { value: "text", label: "Text", group: "input" }, { value: "textarea", label: "Long text", group: "input" },
  { value: "number", label: "Number", group: "input" }, { value: "money", label: "Amount", group: "input" },
  { value: "date", label: "Date", group: "input" }, { value: "select", label: "Dropdown", group: "input" },
  { value: "multiselect", label: "Multi-select", group: "input" }, { value: "radio", label: "Choice", group: "input" },
  { value: "checkbox", label: "Checkbox", group: "input" }, { value: "user", label: "User", group: "input" },
  { value: "org", label: "Organisation", group: "input" }, { value: "file", label: "File upload", group: "input" },
  { value: "table", label: "Table (rows)", group: "input" },
  { value: "section", label: "Section heading", group: "layout" },
  { value: "display", label: "Case value (read-only)", group: "show" },
];
const kindLabel = (k: string) => t(KINDS.find((x) => x.value === k)?.label ?? k);
const FROM_DATA: Record<string, string> = { text: "text", longtext: "textarea", number: "number", money: "money", date: "date",
  boolean: "checkbox", enum: "select", list: "multiselect", user: "user", org: "org", reference: "text" };
const WIDTHS = [{ v: 3, l: "¼" }, { v: 4, l: "⅓" }, { v: 6, l: "½" }, { v: 8, l: "⅔" }, { v: 12, l: "1" }];
const DEVICES = [{ v: "desktop", l: "Desktop" }, { v: "tablet", l: "Tablet" }, { v: "mobile", l: "Mobile" }];
const ident = (s: string) => s.replace(/[^\w]/g, "").replace(/^(\d)/, "_$1");

export default function FormBuilder() {
  const { key } = useParams<{ key: string }>();
  const [f, setF] = useState<Form>({ key, name: "", schema: { fields: [] } });
  const [sel, setSel] = useState(-1);
  const [left, setLeft] = useState<"parts" | "data" | "layers">("parts");
  const [right, setRight] = useState<"props" | "rules" | "checks">("props");
  const [device, setDevice] = useState("desktop");
  const [mode, setMode] = useState<"design" | "try" | "code">("design");
  const [page, setPage] = useState<string | undefined>();
  const [pageEdit, setPageEdit] = useState(false);
  const [tryData, setTryData] = useState<Record<string, any>>({});
  const [tryErr, setTryErr] = useState<Record<string, string>>({});
  const [drag, setDrag] = useState<{ from?: number; kind?: string; data?: DataFieldDef } | null>(null);
  const [over, setOver] = useState<number | null>(null);
  const [e, setE] = useState<string>();
  const [msg, setMsg] = useState<string>();
  const [copied, setCopied] = useState(false);
  useEffect(() => { api.get<Form>(`/api/forms/${key}`).then((x) => setF({ ...x, schema: { fields: x.schema?.fields ?? [], pages: x.schema?.pages } })).catch(() => {}); }, [key]);

  // case data of an app, to bind fields to
  const [apps, setApps] = useState<ProcessDef[]>([]);
  const [app, setApp] = useState("");
  const [data, setData] = useState<DataFieldDef[]>([]);
  useEffect(() => { api.get<ProcessDef[]>("/api/processes").then((x) => { setApps(x ?? []); const guess = (x ?? []).find((p) => key.startsWith(p.key)); if (guess) setApp(guess.key); }).catch(() => {}); }, [key]);
  useEffect(() => {
    if (!app) { setData([]); return; }
    api.get<Version[]>(`/api/processes/${app}/versions`).then((vs) => vs?.[0] && api.get<CaseModel>(`/api/versions/${vs[0].id}/model`))
      .then((m) => setData(m?.data ?? [])).catch(() => setData([]));
  }, [app]);

  const fields = f.schema.fields;
  const pages = f.schema.pages ?? [];
  const curPage = pages.length ? (page && pages.some((p) => p.key === page) ? page : pages[0].key) : undefined;
  const onCur = (x: F) => !curPage || (x.page ?? pages[0].key) === curPage;
  const setSchema = (p: Partial<Form["schema"]>) => { setF({ ...f, schema: { ...f.schema, ...p } }); setMsg(undefined); };
  const setFields = (fs: F[]) => setSchema({ fields: fs });
  const cur = fields[sel];
  const upd = (p: Partial<F>) => setFields(fields.map((x, i) => (i === sel ? { ...x, ...p } : x)));
  const uniqKey = (base: string) => { let k = ident(base) || "field", n = 2; const b = k; while (fields.some((x) => x.key === k)) k = `${b}${n++}`; return k; };
  const fromData = (d: DataFieldDef): F => ({ key: d.key, label: d.label, type: d.calc ? "display" : FROM_DATA[d.type] ?? "text", width: 6,
    required: d.required && !d.requiredIn?.length ? true : undefined, options: d.options, help: d.help, readOnly: !!d.calc || undefined, page: curPage });
  const newField = (kind: string): F => ({ key: uniqKey(kind), label: kindLabel(kind), type: kind,
    width: kind === "section" || kind === "table" || kind === "textarea" ? 12 : 6, page: curPage,
    ...(["select", "multiselect", "radio"].includes(kind) ? { options: [{ value: "a", label: "A" }, { value: "b", label: "B" }] } : {}),
    ...(kind === "table" ? { columns: [{ key: "item", label: t("Item"), type: "text" }, { key: "qty", label: t("Qty"), type: "number" }] } : {}) });
  // index in the full list where an item dropped before visible position `at` goes
  const visible = fields.map((x, i) => ({ x, i })).filter(({ x }) => onCur(x));
  const absAt = (at: number) => (at < visible.length ? visible[at].i : visible.length ? visible[visible.length - 1].i + 1 : fields.length);
  const insert = (fld: F, at: number) => { const n = fields.slice(); n.splice(at, 0, fld); setFields(n); setSel(at); setRight("props"); };
  const dropAt = (vis: number) => {
    if (!drag) return;
    const at = absAt(vis);
    if (drag.from !== undefined) {
      const n = fields.slice(); const [x] = n.splice(drag.from, 1); const to = drag.from < at ? at - 1 : at; n.splice(to, 0, x); setFields(n); setSel(to);
    } else if (drag.data) insert(fromData(drag.data), at);
    else if (drag.kind) insert(newField(drag.kind), at);
    setDrag(null); setOver(null);
  };
  const remove = () => { setFields(fields.filter((_, i) => i !== sel)); setSel(-1); };
  const duplicate = () => { if (!cur) return; const c = { ...cur, key: uniqKey(cur.key), label: cur.label + " (2)" }; insert(c, sel + 1); };

  // pages
  const addPage = () => {
    const first = pages[0]?.key ?? "page1";
    const base = pages.length ? pages : [{ key: first, title: f.name || t("Page") + " 1" }];
    let n = base.length + 1; while (base.some((p) => p.key === `page${n}`)) n++;
    const np: FormPage = { key: `page${n}`, title: `${t("Page")} ${n}` };
    setF({ ...f, schema: { fields: pages.length ? fields : fields.map((x) => ({ ...x, page: x.page ?? first })), pages: [...base, np] } });
    setPage(np.key); setSel(-1);
  };
  const setPageP = (p: Partial<FormPage>) => setSchema({ pages: pages.map((x) => (x.key === curPage ? { ...x, ...p } : x)) });
  const removePage = () => {
    if (pages.length < 2) return;
    const rest = pages.filter((p) => p.key !== curPage);
    setSchema({ pages: rest.length > 1 ? rest : undefined, fields: fields.map((x) => (x.page === curPage || rest.length < 2 ? { ...x, page: rest.length > 1 ? rest[0].key : undefined } : x)) });
    setPage(rest[0].key); setPageEdit(false);
  };

  const compiled = (): Form => ({
    ...f,
    schema: {
      fields: fields.map((x) => ({ ...x, visibleIf: x.show?.rules?.length ? compileCond(x.show) : x.visibleIf })),
      pages: pages.length > 1 ? pages.map((p) => ({ ...p, skipIf: compileCond(p.skip) })) : undefined,
    },
  });
  const save = () => api.put<Form>(`/api/forms/${key}`, compiled())
    .then((x) => { setF({ ...x, schema: { fields: x.schema.fields ?? [], pages: x.schema.pages } }); setMsg(`${t("Saved")} · ${t("revision")} ${x.version}`); setE(undefined); })
    .catch((x) => { setE(x.message); setMsg(undefined); });
  const code = useMemo(() => toTSX(compiled()), [f]); // eslint-disable-line react-hooks/exhaustive-deps
  const others = fields.filter((x, i) => i !== sel && x.type !== "section").map((x) => ({ value: x.key, label: x.label, hint: x.key }));
  const allFieldOpts = fields.filter((x) => x.type !== "section").map((x) => ({ value: x.key, label: x.label, hint: x.key }));

  return (
    <div className="fb">
      <div className="fb-top">
        <input className="fb-name" value={f.name} placeholder={t("Form name")} onChange={(x) => setF({ ...f, name: x.target.value })} aria-label={t("Form name")} />
        <span className="muted small mono">{key}{f.version ? ` · ${t("revision")} ${f.version}` : ""}</span>
        {msg && <span className="small fb-ok">{msg}</span>}
        <span className="fb-sp" />
        <div className="fb-seg" role="group" aria-label="Device">{DEVICES.map((d) => <button key={d.v} className={device === d.v ? "on" : ""} onClick={() => setDevice(d.v)}>{d.l}</button>)}</div>
        <div className="fb-seg" role="group">
          <button className={mode === "design" ? "on" : ""} onClick={() => setMode("design")}>{t("Design")}</button>
          <button className={mode === "try" ? "on" : ""} onClick={() => { setMode("try"); setTryErr({}); }}>{t("Try it")}</button>
          <button className={mode === "code" ? "on" : ""} onClick={() => setMode("code")}>{t("Code")}</button>
        </div>
        <button className="btn" onClick={save}>{t("Save")}</button>
      </div>
      <Err e={e} />
      <div className="fb-body">
        {/* ---------- left ---------- */}
        <aside className="fb-pane">
          <div className="fb-tabs">
            {(["parts", "data", "layers"] as const).map((x) => <button key={x} className={left === x ? "on" : ""} onClick={() => setLeft(x)}>{t({ parts: "Components", data: "Case data", layers: "Layers" }[x])}</button>)}
          </div>
          {left === "parts" && (["input", "layout", "show"] as const).map((g) => (
            <div key={g}>
              <div className="fb-grp">{t({ input: "Inputs", layout: "Layout", show: "Display" }[g])}</div>
              <div className="fb-pal">
                {KINDS.filter((k) => k.group === g).map((k) => (
                  <button key={k.value} draggable onDragStart={() => setDrag({ kind: k.value })} onDragEnd={() => { setDrag(null); setOver(null); }}
                    onClick={() => insert(newField(k.value), absAt(visible.length))}>
                    <span className={"fb-ic fb-ic-" + k.value} aria-hidden />{kindLabel(k.value)}
                  </button>
                ))}
                {g === "layout" && <button onClick={addPage}><span className="fb-ic fb-ic-page" aria-hidden />{t("Page")}</button>}
              </div>
            </div>
          ))}
          {left === "data" && (
            <div>
              <div className="fb-pad"><Dropdown options={apps.map((a) => ({ value: a.key, label: a.name }))} value={app} onChange={setApp} placeholder={t("App")} /></div>
              <ul className="fb-list">
                {data.map((d) => {
                  const used = fields.some((x) => x.key === d.key);
                  return (
                    <li key={d.key} className={used ? "used" : ""} draggable={!used} onDragStart={() => setDrag({ data: d })} onDragEnd={() => { setDrag(null); setOver(null); }}
                      onClick={() => !used && insert(fromData(d), absAt(visible.length))}>
                      <span>{d.label}</span><span className="mono small muted">{d.key}{d.calc ? " ƒ" : ""}</span>
                    </li>
                  );
                })}
                {!data.length && <li className="used">-</li>}
              </ul>
            </div>
          )}
          {left === "layers" && (
            <ul className="fb-list fb-tree">
              {(pages.length ? pages : [{ key: "", title: f.name || t("Page") }]).map((p) => (
                <li key={p.key} className="fb-tree-p">
                  <button className="link" onClick={() => p.key && setPage(p.key)}>{p.title}</button>
                  <ul>
                    {fields.map((x, i) => ({ x, i })).filter(({ x }) => !pages.length || (x.page ?? pages[0].key) === p.key).map(({ x, i }) => (
                      <li key={i} className={(i === sel ? "on " : "") + (x.type === "section" ? "sec" : "")} onClick={() => { setSel(i); if (p.key) setPage(p.key); }}>
                        <span>{x.label}</span><span className="small muted">{x.show?.rules?.length ? t("conditional") : kindLabel(x.type)}</span>
                      </li>
                    ))}
                  </ul>
                </li>
              ))}
            </ul>
          )}
        </aside>

        {/* ---------- canvas ---------- */}
        <main className="fb-canvas">
          {pages.length > 1 && (
            <div className="fb-pages">
              {pages.map((p) => (
                <button key={p.key} className={p.key === curPage ? "on" : ""} onClick={() => { setPage(p.key); setSel(-1); setPageEdit(true); }}>
                  {p.title}{p.skip?.rules?.length ? <span className="fb-flag">{t("conditional")}</span> : null}
                </button>
              ))}
              <button className="fb-addpage" onClick={addPage}>+ {t("Page")}</button>
            </div>
          )}
          {mode === "code" ? (
            <div className="fb-code">
              <div className="fb-code-bar"><span className="small">{f.name || key}.tsx</span>
                <button className="btn-s" onClick={() => { navigator.clipboard?.writeText(code).then(() => setCopied(true)).catch(() => setCopied(false)); }}>{copied ? t("Copied") : t("Copy")}</button></div>
              <pre>{code}</pre>
            </div>
          ) : (
            <div className={"fb-frame fb-" + device}>
              <div className="fb-fhead"><h2>{f.name || key}</h2></div>
              {mode === "try" ? (
                <div className="fb-try">
                  <FormRenderer fields={fields} pages={pages.length > 1 ? pages : undefined} data={tryData} onChange={setTryData} errors={tryErr} />
                  <div className="fb-actions">
                    <button className="btn" onClick={() => { const v = validate(fields, tryData, pages); setTryErr(v); setMsg(Object.keys(v).length ? undefined : t("Form is valid")); }}>{t("Submit")}</button>
                    <button className="btn-s" onClick={() => { setTryData({}); setTryErr({}); }}>{t("Clear")}</button>
                  </div>
                </div>
              ) : (
                <div className="fb-grid" onDragOver={(ev) => { if (drag && !visible.length) { ev.preventDefault(); setOver(0); } }} onDrop={(ev) => { ev.preventDefault(); dropAt(visible.length); }}>
                  {visible.map(({ x, i }, vi) => (
                    <div key={i} style={{ gridColumn: `span ${x.type === "section" ? 12 : device === "mobile" ? x.widthSm || 12 : x.width || 12}` }}
                      className={"fb-f" + (i === sel ? " sel" : "") + (x.show?.rules?.length ? " cond" : "") + (over === vi ? " drop-before" : "") + (x.type === "section" ? " sec" : "")}
                      data-tag={`${kindLabel(x.type)} · ${x.key}`}
                      onClick={(ev) => { ev.stopPropagation(); setSel(i); setPageEdit(false); }}
                      draggable onDragStart={(ev) => { ev.stopPropagation(); setDrag({ from: i }); }} onDragEnd={() => { setDrag(null); setOver(null); }}
                      onDragOver={(ev) => { if (drag) { ev.preventDefault(); ev.stopPropagation(); setOver(vi); } }}
                      onDrop={(ev) => { ev.preventDefault(); ev.stopPropagation(); dropAt(vi); }}>
                      {x.type === "section" ? <h4 className="fsection">{x.label}</h4> : (
                        <>
                          <div className="fld-l">{x.label}{x.required && x.type !== "display" && <span className="req"> *</span>}</div>
                          <div className="fb-inert"><Input f={x} v={x.type === "display" ? "…" : undefined} set={() => {}} ro={x.type === "display"} /></div>
                          {x.help && <div className="muted small">{x.help}</div>}
                        </>
                      )}
                    </div>
                  ))}
                  <div className={"fb-end" + (over === visible.length ? " on" : "")} style={{ gridColumn: "span 12" }}
                    onDragOver={(ev) => { if (drag) { ev.preventDefault(); ev.stopPropagation(); setOver(visible.length); } }}
                    onDrop={(ev) => { ev.preventDefault(); ev.stopPropagation(); dropAt(visible.length); }}>
                    {!visible.length ? t("Drag components or case data here") : ""}
                  </div>
                </div>
              )}
            </div>
          )}
        </main>

        {/* ---------- right ---------- */}
        <aside className="fb-pane fb-right">
          {pageEdit && curPage ? (
            <div className="fb-props">
              <h3>{t("Page")}</h3>
              <Prop label={t("Title")}><input value={pages.find((p) => p.key === curPage)?.title ?? ""} onChange={(x) => setPageP({ title: x.target.value })} /></Prop>
              <h3>{t("Skip this page when")}</h3>
              <CondEditor cond={pages.find((p) => p.key === curPage)?.skip} fields={allFieldOpts} formFields={fields} onChange={(c) => setPageP({ skip: c })} />
              <button className="btn-s danger mt" disabled={pages.length < 2} onClick={removePage}>{t("Remove page")}</button>
            </div>
          ) : cur ? (
            <>
              <div className="fb-tabs">
                {(["props", "rules", "checks"] as const).map((x) => <button key={x} className={right === x ? "on" : ""} onClick={() => setRight(x)}>{t({ props: "Properties", rules: "Conditions", checks: "Validation" }[x])}</button>)}
              </div>
              <div className="fb-props">
                <div className="fb-selbar"><b>{cur.label}</b><span className="fb-sp" />
                  <button className="btn-s" onClick={duplicate}>{t("Duplicate")}</button>
                  <button className="btn-s danger" onClick={remove}>{t("Remove")}</button></div>
                {right === "props" && <>
                  <Prop label={t("Label")}><input value={cur.label} onChange={(x) => upd({ label: x.target.value })} /></Prop>
                  {cur.type !== "section" && <>
                    <Prop label={t("Case data")}>
                      <Dropdown options={data.map((d) => ({ value: d.key, label: d.label, hint: d.key }))} value={data.some((d) => d.key === cur.key) ? cur.key : undefined}
                        placeholder={t("Not linked")} onChange={(k) => { const d = data.find((x) => x.key === k); if (d) upd({ ...fromData(d), width: cur.width, page: cur.page, show: cur.show }); }} />
                    </Prop>
                    <div className="dev-only"><Prop label={t("Variable name")}><input className="mono" value={cur.key} onChange={(x) => upd({ key: ident(x.target.value) })} /></Prop></div>
                    <Prop label={t("Type")}><Dropdown options={KINDS.filter((k) => k.group !== "layout").map((k) => ({ value: k.value, label: kindLabel(k.value) }))} value={cur.type} onChange={(v) => upd({ type: v })} /></Prop>
                  </>}
                  {cur.type !== "section" && <>
                    <Prop label={t("Width")}><Widths value={cur.width ?? 12} onChange={(w) => upd({ width: w })} /></Prop>
                    <Prop label={t("Width on phones")}><Widths value={cur.widthSm ?? 12} onChange={(w) => upd({ widthSm: w === 12 ? undefined : w })} /></Prop>
                    <Toggle label={t("Required")} on={!!cur.required} set={(v) => upd({ required: v || undefined })} />
                    <Toggle label={t("Read-only")} on={!!cur.readOnly} set={(v) => upd({ readOnly: v || undefined })} />
                    {cur.type === "date"
                      ? <Prop label={t("Default value")}><Dropdown options={[{ value: "", label: "—" }, { value: "today", label: t("Today") }]} value={cur.default ?? ""} onChange={(v) => upd({ default: v || undefined })} /></Prop>
                      : !["file", "table", "display", "multiselect"].includes(cur.type) && <Prop label={t("Default value")}><input value={cur.default ?? ""} onChange={(x) => upd({ default: x.target.value || undefined })} /></Prop>}
                    <Prop label={t("Helper text")}><input value={cur.help ?? ""} onChange={(x) => upd({ help: x.target.value || undefined })} /></Prop>
                    {["select", "multiselect", "radio"].includes(cur.type) && <OptionsEditor options={cur.options ?? []} onChange={(o) => upd({ options: o })} />}
                    {cur.type === "table" && <ColumnsEditor cols={cur.columns ?? []} onChange={(c) => upd({ columns: c })} />}
                  </>}
                  {pages.length > 1 && <Prop label={t("Page")}><Dropdown options={pages.map((p) => ({ value: p.key, label: p.title }))} value={cur.page ?? pages[0].key} onChange={(v) => { upd({ page: v }); setPage(v); }} /></Prop>}
                </>}
                {right === "rules" && <>
                  <h3>{t("Show this field when")}</h3>
                  <CondEditor cond={cur.show} fields={others} formFields={fields} onChange={(c) => upd({ show: c, visibleIf: c ? undefined : cur.visibleIf })} />
                  {cur.type !== "section" && <>
                    <h3>{t("Lock this field when")}</h3>
                    <CondEditor cond={cur.lock} fields={others} formFields={fields} onChange={(c) => upd({ lock: c })} />
                  </>}
                  {cur.visibleIf && !cur.show?.rules?.length && <div className="dev-only small mono muted mt">visibleIf: {cur.visibleIf}</div>}
                </>}
                {right === "checks" && <>
                  {(cur.type === "number" || cur.type === "money") && <div className="fb-row2">
                    <Prop label={t("Min")}><input inputMode="decimal" value={cur.min ?? ""} onChange={(x) => upd({ min: x.target.value === "" ? undefined : Number(x.target.value) })} /></Prop>
                    <Prop label={t("Max")}><input inputMode="decimal" value={cur.max ?? ""} onChange={(x) => upd({ max: x.target.value === "" ? undefined : Number(x.target.value) })} /></Prop>
                  </div>}
                  {(cur.type === "text" || cur.type === "textarea") && <>
                    <div className="fb-row2">
                      <Prop label={t("Min characters")}><input inputMode="numeric" value={cur.minLen ?? ""} onChange={(x) => upd({ minLen: x.target.value === "" ? undefined : Number(x.target.value) })} /></Prop>
                      <Prop label={t("Max characters")}><input inputMode="numeric" value={cur.maxLen ?? ""} onChange={(x) => upd({ maxLen: x.target.value === "" ? undefined : Number(x.target.value) })} /></Prop>
                    </div>
                    <Prop label={t("Format")}>
                      <Dropdown options={PATTERNS} value={PATTERNS.find((p) => p.value === cur.pattern)?.value ?? (cur.pattern ? "custom" : "")} allowCustom
                        onChange={(v) => upd({ pattern: v === "" ? undefined : v === "custom" ? cur.pattern ?? "" : v })} />
                    </Prop>
                    {cur.pattern !== undefined && !PATTERNS.some((p) => p.value === cur.pattern) && <div className="dev-only"><Prop label="Regex"><input className="mono" value={cur.pattern} onChange={(x) => upd({ pattern: x.target.value })} /></Prop></div>}
                  </>}
                  {!["number", "money", "text", "textarea"].includes(cur.type) && <div className="muted small">{t("Required is set under Properties")}</div>}
                  <Prop label={t("Message when not valid")}><input value={cur.message ?? ""} onChange={(x) => upd({ message: x.target.value || undefined })} /></Prop>
                </>}
              </div>
            </>
          ) : (
            <div className="fb-props">
              <h3>{t("Form")}</h3>
              <div className="fb-stat"><span>{t("Fields")}</span><span>{fields.filter((x) => x.type !== "section").length}</span></div>
              <div className="fb-stat"><span>{t("Pages")}</span><span>{Math.max(1, pages.length)}</span></div>
              <div className="fb-stat"><span>{t("Conditional fields")}</span><span>{fields.filter((x) => x.show?.rules?.length || x.visibleIf).length}</span></div>
              <button className="btn-s mt" onClick={addPage}>+ {t("Page")}</button>
            </div>
          )}
        </aside>
      </div>
    </div>
  );
}

const PATTERNS = [
  { value: "", label: "—" },
  { value: "\\d{13}", label: "เลขประจำตัวประชาชน 13 หลัก" },
  { value: "0\\d{8,9}", label: "เบอร์โทรศัพท์" },
  { value: "[^@\\s]+@[^@\\s]+\\.[^@\\s]+", label: "อีเมล" },
  { value: "\\d{5}", label: "รหัสไปรษณีย์" },
  { value: "custom", label: "Custom (regex)" },
];

function Prop({ label, children }: { label: string; children: React.ReactNode }) {
  return <label className="fb-prop"><span>{label}</span>{children}</label>;
}
function Toggle({ label, on, set }: { label: string; on: boolean; set: (v: boolean) => void }) {
  return <button type="button" className="fb-toggle" role="switch" aria-checked={on} onClick={() => set(!on)}><span>{label}</span><i className={on ? "on" : ""} /></button>;
}
function Widths({ value, onChange }: { value: number; onChange: (w: number) => void }) {
  return <div className="fb-seg fb-w">{WIDTHS.map((w) => <button type="button" key={w.v} className={value === w.v ? "on" : ""} onClick={() => onChange(w.v)}>{w.l}</button>)}</div>;
}
function OptionsEditor({ options, onChange }: { options: { value: string; label: string }[]; onChange: (o: { value: string; label: string }[]) => void }) {
  const set = (i: number, p: Partial<{ value: string; label: string }>) => onChange(options.map((o, j) => (j === i ? { ...o, ...p } : o)));
  return (
    <div className="fb-prop"><span>{t("Options")}</span>
      <div className="fb-rows">
        {options.map((o, i) => (
          <div key={i} className="fb-orow">
            <input value={o.label} placeholder={t("Label")} onChange={(x) => set(i, { label: x.target.value })} />
            <input className="mono dev-only-inline" value={o.value} placeholder="value" onChange={(x) => set(i, { value: x.target.value })} />
            <button type="button" className="fb-x" aria-label={t("Remove")} onClick={() => onChange(options.filter((_, j) => j !== i))}>×</button>
          </div>
        ))}
        <button type="button" className="link" onClick={() => onChange([...options, { value: `opt${options.length + 1}`, label: `${t("Option")} ${options.length + 1}` }])}>+ {t("Add option")}</button>
      </div>
    </div>
  );
}
function ColumnsEditor({ cols, onChange }: { cols: F[]; onChange: (c: F[]) => void }) {
  const set = (i: number, p: Partial<F>) => onChange(cols.map((c, j) => (j === i ? { ...c, ...p } : c)));
  const types = KINDS.filter((k) => ["text", "number", "money", "date", "select", "checkbox"].includes(k.value)).map((k) => ({ value: k.value, label: kindLabel(k.value) }));
  return (
    <div className="fb-prop"><span>{t("Columns")}</span>
      <div className="fb-rows">
        {cols.map((c, i) => (
          <div key={i} className="fb-orow">
            <input value={c.label} onChange={(x) => set(i, { label: x.target.value, key: c.key || ident(x.target.value) })} />
            <Dropdown options={types} value={c.type} onChange={(v) => set(i, { type: v })} />
            <button type="button" className="fb-x" aria-label={t("Remove")} onClick={() => onChange(cols.filter((_, j) => j !== i))}>×</button>
          </div>
        ))}
        <button type="button" className="link" onClick={() => onChange([...cols, { key: `col${cols.length + 1}`, label: `${t("Column")} ${cols.length + 1}`, type: "text" }])}>+ {t("Add column")}</button>
      </div>
    </div>
  );
}

/** Builds a condition from dropdowns: field, comparison, value. */
function CondEditor({ cond, fields, formFields, onChange }: { cond?: Cond; fields: { value: string; label: string }[]; formFields: F[]; onChange: (c?: Cond) => void }) {
  const rules = cond?.rules ?? [];
  const set = (rs: CondRule[], any = cond?.any) => onChange(rs.length ? { any, rules: rs } : undefined);
  const setRule = (i: number, p: Partial<CondRule>) => set(rules.map((r, j) => (j === i ? { ...r, ...p } : r)));
  return (
    <div className="fb-cond">
      {rules.map((r, i) => {
        const fld = formFields.find((x) => x.key === r.field);
        const needs = OPS.find((o) => o.value === r.op)?.needsValue !== false;
        const valOpts = fld?.type === "checkbox" ? [{ value: "true", label: t("Yes") }, { value: "false", label: t("No") }] : fld?.options?.map((o) => ({ value: String(o.value), label: o.label }));
        return (
          <div key={i}>
            {i > 0 && <button type="button" className="fb-join" onClick={() => set(rules, !cond?.any)}>{cond?.any ? t("or") : t("and")}</button>}
            <div className="fb-rule">
              <Dropdown options={fields} value={r.field} onChange={(v) => setRule(i, { field: v, value: undefined })} placeholder={t("Field")} />
              <Dropdown options={OPS.map((o) => ({ value: o.value, label: t(o.label) }))} value={r.op} onChange={(v) => setRule(i, { op: v as CondRule["op"] })} />
              {needs ? (valOpts?.length
                ? <Dropdown options={valOpts} value={r.value} onChange={(v) => setRule(i, { value: v })} placeholder={t("Value")} />
                : <input value={r.value ?? ""} placeholder={t("Value")} onChange={(x) => setRule(i, { value: x.target.value })} />) : <span />}
              <button type="button" className="fb-x" aria-label={t("Remove")} onClick={() => set(rules.filter((_, j) => j !== i))}>×</button>
            </div>
          </div>
        );
      })}
      <button type="button" className="link" onClick={() => set([...rules, { field: fields[0]?.value ?? "", op: "eq" }])}>+ {t("Add condition")}</button>
    </div>
  );
}

/** React code with the same layout and rules, for developers to take further. */
function toTSX(form: Form) {
  const name = (form.name || form.key).replace(/[^\p{L}\p{N}]+/gu, " ").trim().split(" ").map((w) => w[0]?.toUpperCase() + w.slice(1)).join("").replace(/^[^A-Za-z]+/, "") || "GeneratedForm";
  const comp: Record<string, string> = { text: "TextField", textarea: "TextArea", number: "NumberField", money: "MoneyField", date: "DateField",
    select: "Select", multiselect: "MultiSelect", radio: "Choice", checkbox: "Checkbox", user: "UserPicker", org: "OrgPicker",
    file: "FileDrop", table: "ItemsTable", display: "CaseValue", section: "Section" };
  const attr = (k: string, v: unknown) => (v === undefined || v === false || v === "" ? "" : v === true ? ` ${k}` : typeof v === "number" ? ` ${k}={${v}}` : typeof v === "string" ? ` ${k}=${JSON.stringify(v)}` : ` ${k}={${JSON.stringify(v)}}`);
  const field = (f: F) => f.type === "section" ? `      <Section title=${JSON.stringify(f.label)} />`
    : `      <${comp[f.type] ?? "TextField"} field=${JSON.stringify(f.key)} label=${JSON.stringify(f.label)}${attr("required", f.required)}${attr("span", f.width ?? 12)}${attr("spanSm", f.widthSm)}` +
      `${attr("options", f.options)}${attr("columns", f.columns?.map((c) => ({ key: c.key, label: c.label, type: c.type })))}${attr("min", f.min)}${attr("max", f.max)}${attr("minLength", f.minLen)}${attr("maxLength", f.maxLen)}${attr("pattern", f.pattern)}${attr("message", f.message)}${attr("defaultValue", f.default)}${attr("readOnly", f.readOnly)}` +
      `${f.visibleIf ? ` showWhen={(d) => ${toJs(f.visibleIf)}}` : ""} />`;
  const used = Array.from(new Set(form.schema.fields.map((f) => comp[f.type] ?? "TextField"))).sort();
  const pages = form.schema.pages;
  const body = pages?.length
    ? pages.map((p) => `    <Page title=${JSON.stringify(p.title)}${p.skipIf ? ` skipWhen={(d) => ${toJs(p.skipIf)}}` : ""}>\n${form.schema.fields.filter((f) => (f.page ?? pages[0].key) === p.key).map(field).join("\n")}\n    </Page>`).join("\n")
    : `    <Grid>\n${form.schema.fields.map(field).join("\n")}\n    </Grid>`;
  return `// ${form.name || form.key} — generated from the form builder (key: ${form.key})
import { Form, Grid, Page, ${used.join(", ")} } from "@cm/ui";
import type { CaseData } from "@cm/ui";

export default function ${name}({ data, onSubmit }: { data: CaseData; onSubmit: (d: CaseData) => void }) {
  return (
    <Form data={data} onSubmit={onSubmit}>
${body}
    </Form>
  );
}
`;
}
const toJs = (expr: string) => expr.replace(/\b([A-Za-z_]\w*)\b(?=\s*(==|!=|>|<|\?\?))/g, "d.$1").replace(/\(\s*([A-Za-z_]\w*)\s*\?\?/g, "(d.$1 ??").replace(/d\.d\./g, "d.");

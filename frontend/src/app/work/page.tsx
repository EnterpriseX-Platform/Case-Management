"use client";
// Work list with saved views: pick columns (including case data), filter,
// group, sort, density and highlight rules. Views are personal, or shared by
// a supervisor with roles (optionally as their default).
import { t } from "@/lib/i18n";
import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { api, DataFieldDef, ProcessDef, Work } from "@/lib/api";
import { dt, fieldValue, loopLabel, overdue } from "@/lib/format";
import { useLoad } from "@/lib/useLoad";
import { has, useSession } from "@/lib/useSession";
import { useDialog } from "@/components/useDialog";
import Dropdown from "@/components/Dropdown";
import { Err, Status } from "@/components/ui";

type Rule = { field: string; op: string; value?: string };
type View = {
  id?: string; name: string; owner?: string; shared?: boolean; roles?: string[]; default?: boolean; scope?: string;
  columns: string[]; filters?: Rule[]; groupBy?: string; sortBy?: string; sortDesc?: boolean; density?: string;
  highlight?: { when: Rule[]; color: string }[]; builtin?: boolean;
};
type Row = { work: Work; data: Record<string, any> | null };

const COLS: { key: string; label: string; num?: boolean }[] = [
  { key: "urgency", label: "Urgency", num: true }, { key: "caseNumber", label: "Case no." }, { key: "caseTitle", label: "Case" },
  { key: "step", label: "Step" }, { key: "caseType", label: "Case type" }, { key: "stage", label: "Stage" },
  { key: "role", label: "Role / queue" }, { key: "assignee", label: "Assignee" }, { key: "received", label: "Received" },
  { key: "due", label: "Due" }, { key: "status", label: "Status" },
];
const DEFAULT_COLS = ["urgency", "caseNumber", "caseTitle", "step", "role", "received", "due", "status"];
const BUILTIN: View[] = [
  { id: "builtin:mine", name: "My work", scope: "mine", columns: DEFAULT_COLS, builtin: true },
  { id: "builtin:queue", name: "Role queue", scope: "queue", columns: DEFAULT_COLS, builtin: true },
  { id: "builtin:all", name: "Everything I can see", scope: "all", columns: DEFAULT_COLS, builtin: true },
];
const OPS = [
  { value: "eq", label: "=" }, { value: "ne", label: "≠" }, { value: "contains", label: "contains" },
  { value: "gt", label: ">" }, { value: "lt", label: "<" }, { value: "empty", label: "is empty" }, { value: "filled", label: "is filled" },
];
const COLORS = [{ value: "red", label: "Red" }, { value: "amber", label: "Amber" }, { value: "blue", label: "Blue" }, { value: "green", label: "Green" }];

export default function Inbox() {
  const s = useSession();
  const r = useRouter();
  const dlg = useDialog();
  const sup = !!s && has(s, "cm:supervisor");
  const { data: saved, reload: reloadViews } = useLoad(() => api.get<View[]>("/api/views").catch(() => [] as View[]), [s?.id]);
  const { data: dataFields } = useLoad(() => api.get<DataFieldDef[]>("/api/work/fields").catch(() => [] as DataFieldDef[]));
  const { data: procs } = useLoad(() => api.get<ProcessDef[]>("/api/processes").catch(() => [] as ProcessDef[]));
  const [view, setView] = useState<View | null>(null);
  const [dirty, setDirty] = useState(false);
  const [panel, setPanel] = useState<"" | "columns" | "filter" | "highlight" | "share">("");
  const [q, setQ] = useState("");
  const [e, setE] = useState<string>();
  const [msg, setMsg] = useState<string>();

  // first view: a shared default for my roles, else My work
  useEffect(() => {
    if (view || !saved) return;
    setView(saved.find((v) => v.shared && v.default) ?? BUILTIN[0]);
  }, [saved, view]);
  const v = view ?? BUILTIN[0];
  const dataCols = v.columns.filter((c) => c.startsWith("data.")).map((c) => c.slice(5));
  const filterData = (v.filters ?? []).map((f) => f.field).filter((f) => f.startsWith("data.")).map((f) => f.slice(5));
  const wantFields = Array.from(new Set([...dataCols, ...filterData, ...(v.highlight ?? []).flatMap((h) => h.when.map((w) => w.field)).filter((f) => f.startsWith("data.")).map((f) => f.slice(5))])).sort().join(",");
  const { data: rows, error, reload } = useLoad(() => api.get<Row[]>(`/api/worklist?fields=${encodeURIComponent(wantFields)}${v.scope === "all" && sup ? "&all=1" : ""}`), [s?.id, wantFields, v.scope]);

  const set = (p: Partial<View>) => { setView({ ...v, ...p }); setDirty(true); setMsg(undefined); };
  const fieldDef = (k: string) => dataFields?.find((f) => f.key === k);
  const procName = (k?: string) => procs?.find((p) => p.key === k)?.name ?? k ?? "";
  const colLabel = (c: string) => c.startsWith("data.") ? fieldDef(c.slice(5))?.label ?? c.slice(5) : t(COLS.find((x) => x.key === c)?.label ?? c);

  // value of a column / filter field for a row
  const val = (row: Row, c: string): any => {
    const w = row.work;
    switch (c) {
      case "urgency": return w.urgency ?? 0;
      case "caseNumber": return w.caseNumber;
      case "caseTitle": return w.caseTitle;
      case "step": return w.name;
      case "caseType": return procName(row.data?._processKey);
      case "stage": return row.data?._stage ?? "";
      case "role": return w.queue ? `${t("Queue")} ${w.queue}` : w.role ?? "";
      case "assignee": return w.assignee ?? "";
      case "received": return w.createdAt;
      case "due": return w.dueAt ?? "";
      case "status": return w.status;
      case "sla": return overdue(w.dueAt) ? "overdue" : w.goalAt && new Date(w.goalAt) < new Date() ? "atrisk" : "ontime";
      default: return c.startsWith("data.") ? row.data?.[c.slice(5)] : undefined;
    }
  };
  const test = (row: Row, f: Rule) => {
    const x = val(row, f.field), want = f.value ?? "";
    const empty = x === undefined || x === null || x === "";
    switch (f.op) {
      case "empty": return empty;
      case "filled": return !empty;
      case "contains": return String(x ?? "").toLowerCase().includes(want.toLowerCase());
      case "gt": return !empty && Number(x) > Number(want);
      case "lt": return !empty && Number(x) < Number(want);
      case "ne": return String(x ?? "") !== want;
      default: return String(x ?? "") === want;
    }
  };

  const list = useMemo(() => {
    let l = (rows ?? []).filter((row) => v.scope === "mine" ? row.work.assignee === s?.id : v.scope === "queue" ? !row.work.assignee : true);
    l = l.filter((row) => (v.filters ?? []).every((f) => !f.field || test(row, f)));
    if (q.trim()) { const n = q.toLowerCase(); l = l.filter((row) => [row.work.caseNumber, row.work.caseTitle, row.work.name, ...dataCols.map((c) => row.data?.[c])].some((x) => String(x ?? "").toLowerCase().includes(n))); }
    if (v.sortBy) {
      const k = v.sortBy, d = v.sortDesc ? -1 : 1;
      l = l.slice().sort((a, b) => { const x = val(a, k), y = val(b, k); return (typeof x === "number" && typeof y === "number" ? x - y : String(x ?? "").localeCompare(String(y ?? ""))) * d; });
    }
    return l;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rows, v, q, s?.id, procs]);
  const groups = useMemo(() => {
    if (!v.groupBy) return [{ key: "", rows: list }];
    const m = new Map<string, Row[]>();
    for (const row of list) { const k = String(cell(v.groupBy, row) || "—"); m.set(k, [...(m.get(k) ?? []), row]); }
    return Array.from(m, ([key, rows]) => ({ key, rows }));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [list, v.groupBy]);
  function cell(c: string, row: Row) {
    const x = val(row, c);
    if (c === "received" || c === "due") return dt(x || undefined);
    if (c.startsWith("data.")) return fieldValue(x, fieldDef(c.slice(5)) as any);
    return x;
  }
  const rowClass = (row: Row) => {
    const hl = (v.highlight ?? []).find((h) => h.when.length && h.when.every((w) => !w.field || test(row, w)));
    if (hl) return "hl-" + hl.color;
    return overdue(row.work.dueAt) ? "late" : val(row, "sla") === "atrisk" ? "risk" : "";
  };

  const allFields = [
    ...COLS.map((c) => ({ value: c.key, label: t(c.label) })), { value: "sla", label: "SLA" },
    ...(dataFields ?? []).map((f) => ({ value: "data." + f.key, label: f.label, hint: t("Case data") })),
  ];
  const valueOpts = (field: string) => {
    if (field === "sla") return [{ value: "overdue", label: t("Overdue") }, { value: "atrisk", label: t("Past goal") }, { value: "ontime", label: t("On time") }];
    if (field === "status") return [{ value: "open", label: t("Open") }, { value: "claimed", label: t("Claimed") }];
    if (field === "caseType") return (procs ?? []).map((p) => ({ value: p.name, label: p.name }));
    if (field.startsWith("data.")) return fieldDef(field.slice(5))?.options?.map((o) => ({ value: String(o.value), label: o.label }));
    return undefined;
  };
  const ruleLabel = (f: Rule) => `${allFields.find((x) => x.value === f.field)?.label ?? f.field} ${t(OPS.find((o) => o.value === f.op)?.label ?? f.op)} ${["empty", "filled"].includes(f.op) ? "" : valueOpts(f.field)?.find((o) => o.value === f.value)?.label ?? f.value ?? ""}`;

  const save = async (asNew: boolean) => {
    let name = v.name;
    if (asNew || v.builtin || (v.shared && !sup)) {
      const n = await dlg.prompt(t("Save view"), t("View name"), v.builtin ? "" : v.name, { ok: t("Save") });
      if (!n) return;
      name = n;
    }
    const body: View = { ...v, name, builtin: undefined, id: undefined, owner: undefined, shared: asNew || v.builtin ? false : v.shared };
    const id = asNew || v.builtin || (v.shared && !sup) ? "new" : v.id!;
    api.put<View>(`/api/views/${id}`, body).then((x) => { setView(x); setDirty(false); setMsg(t("Saved")); reloadViews(); }).catch((x) => setE(x.message));
  };
  const remove = async () => {
    if (!v.id || v.builtin) return;
    if (await dlg.confirm(`${t("Delete view")}: ${v.name}`, { ok: t("Delete"), danger: true })) {
      api.del(`/api/views/${v.id}`).then(() => { setView(BUILTIN[0]); reloadViews(); }).catch((x) => setE(x.message));
    }
  };
  const mineViews = (saved ?? []).filter((x) => !x.shared);
  const teamViews = (saved ?? []).filter((x) => x.shared);
  const canEdit = !v.builtin && (!v.shared || sup);

  return (
    <div className="wq">
      {dlg.node}
      <aside className="wq-side">
        <div className="wq-grp">{t("Views")}</div>
        <ul className="wq-views">{BUILTIN.map((x) => <ViewItem key={x.id} v={x} on={v.id === x.id} pick={() => { setView(x); setDirty(false); }} />)}</ul>
        {mineViews.length > 0 && <><div className="wq-grp">{t("My views")}</div>
          <ul className="wq-views">{mineViews.map((x) => <ViewItem key={x.id} v={x} on={v.id === x.id} pick={() => { setView(x); setDirty(false); }} />)}</ul></>}
        {teamViews.length > 0 && <><div className="wq-grp">{t("Team views")}</div>
          <ul className="wq-views">{teamViews.map((x) => <ViewItem key={x.id} v={x} on={v.id === x.id} pick={() => { setView(x); setDirty(false); }} />)}</ul></>}
      </aside>

      <main className="wq-main">
        <div className="wq-head">
          <h1>{t(v.name)}{dirty && <span className="muted small"> · {t("changed")}</span>}</h1>
          <span className="wq-sp" />
          {msg && <span className="small wq-ok">{msg}</span>}
          {dirty && canEdit && <button className="btn" onClick={() => save(false)}>{t("Save view")}</button>}
          {(dirty || v.builtin) && <button className="btn-o" onClick={() => save(true)}>{t("Save as new view")}</button>}
          <button className="btn" onClick={() => api.post<Work>("/api/work/next").then((w) => r.push(`/work/${w.id}`)).catch((x) => setE(x.status === 404 ? t("No work waiting in your queues") : x.message))}>{t("Get next work")}</button>
        </div>
        <Err e={error ?? e} />

        <div className="wq-tools">
          <input className="wq-search" value={q} onChange={(x) => setQ(x.target.value)} placeholder={t("Search case no., title, step")} aria-label={t("Search")} />
          <button className={"btn-s" + (panel === "filter" ? " on" : "")} onClick={() => setPanel(panel === "filter" ? "" : "filter")}>+ {t("Filter")}</button>
          <div className="wq-pick"><span className="small muted">{t("Group by")}</span>
            <Dropdown options={[{ value: "", label: "—" }, ...allFields.filter((f) => !["received", "due", "urgency"].includes(f.value))]} value={v.groupBy ?? ""} onChange={(x) => set({ groupBy: x || undefined })} /></div>
          <div className="wq-pick"><span className="small muted">{t("Sort by")}</span>
            <Dropdown options={[{ value: "", label: t("Urgency") }, ...allFields]} value={v.sortBy ?? ""} onChange={(x) => set({ sortBy: x || undefined })} />
            <button className="btn-s" aria-label={t("Sort direction")} onClick={() => set({ sortDesc: !v.sortDesc })}>{v.sortDesc ? "↓" : "↑"}</button></div>
          <span className="wq-sp" />
          <div className="fb-seg">{[{ v: "", l: t("Normal") }, { v: "compact", l: t("Compact") }].map((d) => <button key={d.v} className={(v.density ?? "") === d.v ? "on" : ""} onClick={() => set({ density: d.v || undefined })}>{d.l}</button>)}</div>
          <button className={"btn-s" + (panel === "columns" ? " on" : "")} onClick={() => setPanel(panel === "columns" ? "" : "columns")}>{t("Columns")}</button>
          <button className={"btn-s" + (panel === "highlight" ? " on" : "")} onClick={() => setPanel(panel === "highlight" ? "" : "highlight")}>{t("Highlight")}</button>
          {!v.builtin && (sup || !v.shared) && <button className={"btn-s" + (panel === "share" ? " on" : "")} onClick={() => setPanel(panel === "share" ? "" : "share")}>{t("Sharing")}</button>}
        </div>

        {(v.filters ?? []).length > 0 && (
          <div className="wq-chips">
            {(v.filters ?? []).map((f, i) => <span key={i} className="wq-chip">{ruleLabel(f)}<button aria-label={t("Remove")} onClick={() => set({ filters: (v.filters ?? []).filter((_, j) => j !== i) })}>×</button></span>)}
            <button className="link small" onClick={() => set({ filters: [] })}>{t("Clear filters")}</button>
          </div>
        )}

        {panel === "filter" && <RulePanel title={t("Add filter")} fields={allFields} valueOpts={valueOpts} onAdd={(f) => { set({ filters: [...(v.filters ?? []), f] }); setPanel(""); }} />}
        {panel === "columns" && (
          <div className="wq-panel">
            <div className="wq-cols">
              {v.columns.map((c, i) => (
                <div key={c} className="wq-col">
                  <button className="wq-mv" aria-label={t("Move up")} disabled={i === 0} onClick={() => { const n = v.columns.slice(); [n[i - 1], n[i]] = [n[i], n[i - 1]]; set({ columns: n }); }}>↑</button>
                  <button className="wq-mv" aria-label={t("Move down")} disabled={i === v.columns.length - 1} onClick={() => { const n = v.columns.slice(); [n[i + 1], n[i]] = [n[i], n[i + 1]]; set({ columns: n }); }}>↓</button>
                  <span>{colLabel(c)}</span>
                  <button className="fb-x" aria-label={t("Remove")} disabled={v.columns.length < 2} onClick={() => set({ columns: v.columns.filter((x) => x !== c) })}>×</button>
                </div>
              ))}
            </div>
            <div className="wq-add">
              <span className="small muted">{t("Add column")}</span>
              <Dropdown options={[...COLS.map((c) => ({ value: c.key, label: t(c.label) })), ...(dataFields ?? []).map((f) => ({ value: "data." + f.key, label: f.label, hint: t("Case data") }))].filter((o) => !v.columns.includes(o.value))}
                value="" placeholder={t("Choose a column")} onChange={(x) => x && set({ columns: [...v.columns, x] })} />
            </div>
          </div>
        )}
        {panel === "highlight" && (
          <div className="wq-panel">
            <div className="small muted">{t("Overdue rows are red and rows past their goal are amber unless a rule below matches first.")}</div>
            {(v.highlight ?? []).map((h, i) => (
              <div key={i} className="wq-hl"><span className={"wq-dot hl-" + h.color} />{h.when.map(ruleLabel).join(` ${t("and")} `)}
                <button className="fb-x" aria-label={t("Remove")} onClick={() => set({ highlight: (v.highlight ?? []).filter((_, j) => j !== i) })}>×</button></div>
            ))}
            <RulePanel title={t("Add highlight rule")} fields={allFields} valueOpts={valueOpts} withColor onAdd={(f, color) => set({ highlight: [...(v.highlight ?? []), { when: [f], color: color ?? "blue" }] })} />
          </div>
        )}
        {panel === "share" && (
          <div className="wq-panel">
            {sup ? <>
              <button type="button" className="fb-toggle" onClick={() => set({ shared: !v.shared })}><span>{t("Share with my team")}</span><i className={v.shared ? "on" : ""} /></button>
              {v.shared && <>
                <div className="wq-add"><span className="small muted">{t("Roles that see this view (empty = everyone)")}</span>
                  <Dropdown multiple allowCustom options={Array.from(new Set([...(s?.roles ?? []), ...(saved ?? []).flatMap((x) => x.roles ?? [])])).map((x) => ({ value: x, label: x }))} value={v.roles ?? []} onChange={(x: any) => set({ roles: x })} /></div>
                <button type="button" className="fb-toggle" onClick={() => set({ default: !v.default })}><span>{t("Default view for these roles")}</span><i className={v.default ? "on" : ""} /></button>
              </>}
            </> : <div className="small muted">{t("Only supervisors can share views.")}</div>}
            {!v.builtin && <button className="btn-s danger mt" onClick={remove}>{t("Delete view")}</button>}
          </div>
        )}

        {list.length ? (
          <div className={"wq-scroll" + (v.density === "compact" ? " compact" : "")}>
            <table className="tbl wq-tbl">
              <thead><tr>{v.columns.map((c) => (
                <th key={c} className={COLS.find((x) => x.key === c)?.num || fieldDef(c.slice(5))?.type === "money" || fieldDef(c.slice(5))?.type === "number" ? "r" : ""}>
                  <button className="wq-th" onClick={() => set(v.sortBy === c ? { sortDesc: !v.sortDesc } : { sortBy: c, sortDesc: false })}>{colLabel(c)}{v.sortBy === c ? (v.sortDesc ? " ↓" : " ↑") : ""}</button>
                </th>))}<th /></tr></thead>
              <tbody>
                {groups.map((g) => [
                  g.key ? <tr key={"g" + g.key} className="wq-g"><td colSpan={v.columns.length + 1}>{g.key} · {g.rows.length}</td></tr> : null,
                  ...g.rows.map((row) => (
                    <tr key={row.work.id} className={rowClass(row)}>
                      {v.columns.map((c) => {
                        const num = COLS.find((x) => x.key === c)?.num || ["money", "number"].includes(fieldDef(c.slice(5))?.type ?? "");
                        return (
                          <td key={c} className={(num ? "r " : "") + (c === "caseTitle" ? "" : "nw")}>
                            {c === "step" ? <><Link href={`/work/${row.work.id}`}>{row.work.name}</Link><span className="muted">{loopLabel(row.work.input)}</span></>
                              : c === "status" ? <Status s={row.work.status} />
                              : c === "due" ? <>{cell(c, row)}{overdue(row.work.dueAt) && <span className="late-t"> {t("Overdue")}</span>}</>
                              : cell(c, row)}
                          </td>
                        );
                      })}
                      <td className="nw">{!row.work.assignee
                        ? <button className="btn-s" onClick={() => api.post(`/api/work/${row.work.id}/claim`).then(reload).catch((x) => setE(x.message))}>{t("Claim")}</button>
                        : <Link className="btn-s" href={`/work/${row.work.id}`}>{t("Open")}</Link>}</td>
                    </tr>
                  )),
                ])}
              </tbody>
            </table>
          </div>
        ) : rows && <div className="wq-empty">{t("No work items")}</div>}
        <div className="small muted mt">{list.length} {t("items")}</div>
      </main>
    </div>
  );
}

function ViewItem({ v, on, pick }: { v: View; on: boolean; pick: () => void }) {
  return <li><button className={on ? "on" : ""} onClick={pick}>{t(v.name)}{v.default ? <span className="small muted"> · {t("default")}</span> : null}</button></li>;
}

function RulePanel({ title, fields, valueOpts, onAdd, withColor }: {
  title: string; fields: { value: string; label: string }[]; valueOpts: (f: string) => { value: string; label: string }[] | undefined;
  onAdd: (r: Rule, color?: string) => void; withColor?: boolean;
}) {
  const [r, setR] = useState<Rule>({ field: "", op: "eq" });
  const [color, setColor] = useState("blue");
  const opts = r.field ? valueOpts(r.field) : undefined;
  const needs = !["empty", "filled"].includes(r.op);
  return (
    <div className="wq-panel wq-rule">
      <span className="small muted">{title}</span>
      <div className="wq-rule-row">
        <Dropdown options={fields} value={r.field} onChange={(x) => setR({ ...r, field: x, value: undefined })} placeholder={t("Field")} />
        <Dropdown options={OPS.map((o) => ({ value: o.value, label: t(o.label) }))} value={r.op} onChange={(x) => setR({ ...r, op: x })} />
        {needs ? (opts?.length ? <Dropdown options={opts} value={r.value} onChange={(x) => setR({ ...r, value: x })} placeholder={t("Value")} />
          : <input value={r.value ?? ""} placeholder={t("Value")} onChange={(x) => setR({ ...r, value: x.target.value })} />) : <span />}
        {withColor && <Dropdown options={COLORS.map((c) => ({ ...c, label: t(c.label) }))} value={color} onChange={setColor} />}
        <button className="btn-s" disabled={!r.field || (needs && !r.value)} onClick={() => { onAdd(r, color); setR({ field: "", op: "eq" }); }}>{t("Add")}</button>
      </div>
    </div>
  );
}

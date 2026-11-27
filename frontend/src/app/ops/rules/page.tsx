"use client";
// Work distribution rules: supervisors decide who gets new work without
// code. Rules are checked from the top; the first that matches decides.
import { t } from "@/lib/i18n";
import { useEffect, useMemo, useState } from "react";
import { api, DataFieldDef, ProcessDef, Queue, UserProfile } from "@/lib/api";
import { useLoad } from "@/lib/useLoad";
import { useDialog } from "@/components/useDialog";
import Dropdown from "@/components/Dropdown";
import { Err, Page } from "@/components/ui";

type Cond = { field: string; op: string; value?: string };
type Action = { mode: string; queue?: string; role?: string; user?: string; skill?: string; maxPerPerson?: number; urgency?: number; sla?: string; goal?: string };
type Rule = { id?: string; name: string; order?: number; active: boolean; when: Cond[]; any?: boolean; then: Action };
type Step = { caseType: string; step: string; name: string; role?: string };
type Preview = { rule?: string; assignee?: string; queue?: string; role?: string; urgency: number; sla?: string; pool?: string[]; note?: string };

const MODES = [
  { value: "workload", label: "Least busy person in a queue" }, { value: "roundrobin", label: "Next person in turn (queue)" },
  { value: "skill", label: "Person with a skill (queue)" }, { value: "queue", label: "Into a queue (people pick it up)" },
  { value: "role", label: "Anyone with a role" }, { value: "user", label: "A specific person" },
  { value: "keep", label: "Keep the step's own routing" },
];
const OPS = [
  { value: "eq", label: "=" }, { value: "ne", label: "≠" }, { value: "in", label: "is one of" }, { value: "contains", label: "contains" },
  { value: "gt", label: ">" }, { value: "gte", label: "≥" }, { value: "lt", label: "<" }, { value: "lte", label: "≤" },
  { value: "empty", label: "is empty" }, { value: "filled", label: "is filled" },
];
const DURATIONS = ["PT4H", "PT8H", "P1D", "P2D", "P3D", "P5D", "P7D"].map((d) => ({ value: d, label: d }));
const blank = (): Rule => ({ name: "", active: true, when: [], then: { mode: "workload" } });

export default function RulesPage() {
  const dlg = useDialog();
  const { data: saved, reload, error } = useLoad(() => api.get<Rule[]>("/api/assign-rules"));
  const { data: procs } = useLoad(() => api.get<ProcessDef[]>("/api/processes"));
  const { data: steps } = useLoad(() => api.get<Step[]>("/api/work/steps").catch(() => [] as Step[]));
  const { data: fields } = useLoad(() => api.get<DataFieldDef[]>("/api/work/fields").catch(() => [] as DataFieldDef[]));
  const { data: queues } = useLoad(() => api.get<Queue[]>("/api/queues").catch(() => [] as Queue[]));
  const { data: users } = useLoad(() => api.get<UserProfile[]>("/api/users").catch(() => [] as UserProfile[]));
  const [rules, setRules] = useState<Rule[]>([]);
  const [sel, setSel] = useState<number>(-1);
  const [dirty, setDirty] = useState(false);
  const [e, setE] = useState<string>();
  const [msg, setMsg] = useState<string>();
  useEffect(() => { if (saved) { setRules(saved); setSel((x) => (x < 0 && saved.length ? 0 : x)); } }, [saved]);
  const cur = rules[sel];
  const upd = (p: Partial<Rule>) => { setRules(rules.map((r, i) => (i === sel ? { ...r, ...p } : r))); setDirty(true); setMsg(undefined); };
  const updThen = (p: Partial<Action>) => cur && upd({ then: { ...cur.then, ...p } });

  const procName = (k?: string) => procs?.find((p) => p.key === k)?.name ?? k ?? "";
  const roles = useMemo(() => Array.from(new Set([...(users ?? []).flatMap((u) => u.roles ?? []), ...(queues ?? []).flatMap((q) => q.roles ?? []), ...(steps ?? []).map((s) => s.role ?? "")]
    .filter((r) => r && !r.startsWith("queue:") && !r.startsWith("cm:")))).sort(), [users, queues, steps]);
  const skills = useMemo(() => Array.from(new Set((users ?? []).flatMap((u) => u.skills ?? []))).sort(), [users]);
  const fieldOpts = [
    { value: "caseType", label: t("Case type") }, { value: "step", label: t("Step") }, { value: "stage", label: t("Stage") },
    { value: "role", label: t("Role of the step") }, { value: "urgency", label: t("Urgency") },
    ...(fields ?? []).map((f) => ({ value: "data." + f.key, label: f.label, hint: t("Case data") })),
  ];
  const caseTypeIn = (r?: Rule) => r?.when.find((c) => c.field === "caseType" && c.op === "eq")?.value;
  const valueOpts = (field: string, r?: Rule) => {
    if (field === "caseType") return (procs ?? []).map((p) => ({ value: p.key, label: p.name }));
    if (field === "step") { const ct = caseTypeIn(r); return (steps ?? []).filter((s) => !ct || s.caseType === ct).map((s) => ({ value: s.step, label: s.name, hint: ct ? undefined : procName(s.caseType) })); }
    if (field === "role") return roles.map((x) => ({ value: x, label: x }));
    if (field.startsWith("data.")) return fields?.find((f) => "data." + f.key === field)?.options?.map((o) => ({ value: String(o.value), label: o.label }));
    return undefined;
  };
  const condText = (c: Cond, r?: Rule) => {
    const f = fieldOpts.find((x) => x.value === c.field)?.label ?? c.field;
    const op = t(OPS.find((o) => o.value === c.op)?.label ?? c.op);
    const v = ["empty", "filled"].includes(c.op) ? "" : valueOpts(c.field, r)?.find((o) => o.value === c.value)?.label ?? c.value ?? "";
    return `${f} ${op} ${v}`.trim();
  };
  const thenText = (a: Action) => {
    const q = queues?.find((x) => x.key === a.queue)?.name ?? a.queue;
    const who = a.mode === "workload" ? `${t("least busy in")} ${q}` : a.mode === "roundrobin" ? `${t("in turn in")} ${q}` : a.mode === "skill" ? `${t("skill")} ${a.skill} ${t("in")} ${q}`
      : a.mode === "queue" ? `${t("queue")} ${q}` : a.mode === "role" ? `${t("role")} ${a.role}` : a.mode === "user" ? a.user : t("step's own routing");
    return [who, a.maxPerPerson ? `${t("max")} ${a.maxPerPerson}/${t("person")}` : "", a.urgency ? `${t("urgency")} ${a.urgency > 0 ? "+" : ""}${a.urgency}` : "", a.sla ? `SLA ${a.sla}` : ""].filter(Boolean).join(" · ");
  };

  const add = () => { const n = [...rules, { ...blank(), name: `${t("Rule")} ${rules.length + 1}` }]; setRules(n); setSel(n.length - 1); setDirty(true); };
  const move = (i: number, d: number) => {
    const j = i + d; if (j < 0 || j >= rules.length) return;
    const n = rules.slice(); [n[i], n[j]] = [n[j], n[i]]; setRules(n); setSel(j); setDirty(true);
  };
  const saveAll = async () => {
    setE(undefined);
    try {
      const out: Rule[] = [];
      for (let i = 0; i < rules.length; i++) out.push(await api.put<Rule>(`/api/assign-rules/${rules[i].id ?? "new"}`, { ...rules[i], order: i + 1 }));
      const keep = new Set(out.map((r) => r.id));
      for (const r of saved ?? []) if (r.id && !keep.has(r.id)) await api.del(`/api/assign-rules/${r.id}`);
      await api.post("/api/assign-rules/order", { ids: out.map((r) => r.id) });
      setDirty(false); setMsg(t("Rules published")); reload();
    } catch (x: any) { setE(x.message); }
  };
  const remove = async () => {
    if (!cur) return;
    if (await dlg.confirm(`${t("Delete rule")}: ${cur.name}`, { ok: t("Delete"), danger: true })) {
      setRules(rules.filter((_, i) => i !== sel)); setSel(Math.max(0, sel - 1)); setDirty(true);
    }
  };

  // dry run with the rules as they are on screen (not yet published)
  const [test, setTest] = useState<{ caseType: string; step: string; data: { k: string; v: string }[] }>({ caseType: "", step: "", data: [] });
  const [pv, setPv] = useState<Preview | null>(null);
  const runTest = () => {
    const data: Record<string, unknown> = {};
    for (const d of test.data) if (d.k) data[d.k] = d.v !== "" && !isNaN(Number(d.v)) ? Number(d.v) : d.v === "true" ? true : d.v === "false" ? false : d.v;
    const st = steps?.find((s) => s.caseType === test.caseType && s.step === test.step);
    api.post<Preview>("/api/assign-rules/preview", { caseType: test.caseType, step: test.step, role: st?.role ?? "", data, rules: rules.map((r, i) => ({ ...r, order: i + 1 })) })
      .then(setPv).catch((x) => setE(x.message));
  };

  return (
    <Page title={t("Work distribution")} actions={<>
      {msg && <span className="small th-ok">{msg}</span>}
      <button className="btn-o" onClick={add}>+ {t("Rule")}</button>
      <button className="btn" disabled={!dirty} onClick={saveAll}>{t("Publish rules")}</button>
    </>}>
      {dlg.node}
      <Err e={error ?? e} />
      <div className="ar">
        <section className="ar-list">
          <div className="ar-head small muted">{t("Checked from the top. The first rule that matches decides.")}</div>
          {rules.map((r, i) => (
            <div key={r.id ?? "n" + i} className={"ar-item" + (i === sel ? " on" : "") + (r.active ? "" : " off")} onClick={() => setSel(i)}>
              <div className="ar-ord">{i + 1}</div>
              <div className="ar-body">
                <div className="ar-name">{r.name || "—"}{!r.active && <span className="small muted"> · {t("paused")}</span>}</div>
                <div className="small"><span className="muted">{t("When")}</span> {r.when.length ? r.when.map((c) => condText(c, r)).join(r.any ? ` ${t("or")} ` : ` ${t("and")} `) : t("any new work")}</div>
                <div className="small"><span className="muted">{t("Give to")}</span> {thenText(r.then)}</div>
              </div>
              <div className="ar-mv">
                <button aria-label={t("Move up")} disabled={i === 0} onClick={(x) => { x.stopPropagation(); move(i, -1); }}>↑</button>
                <button aria-label={t("Move down")} disabled={i === rules.length - 1} onClick={(x) => { x.stopPropagation(); move(i, 1); }}>↓</button>
              </div>
            </div>
          ))}
          {!rules.length && saved && <div className="ar-empty">{t("No rules yet. Each step uses the routing set in Studio.")}<div><button className="btn-s mt" onClick={add}>+ {t("Rule")}</button></div></div>}
          <div className="ar-item ar-fallback"><div className="ar-ord">—</div><div className="ar-body"><div className="ar-name">{t("Otherwise")}</div><div className="small muted">{t("the step's own routing from Studio")}</div></div></div>
        </section>

        <section className="ar-edit">
          {cur ? <>
            <div className="ar-row">
              <label className="fb-prop ar-grow"><span>{t("Rule name")}</span><input value={cur.name} onChange={(x) => upd({ name: x.target.value })} /></label>
              <button type="button" className="fb-toggle ar-tg" onClick={() => upd({ active: !cur.active })}><span>{t("Active")}</span><i className={cur.active ? "on" : ""} /></button>
            </div>
            <h3>{t("When new work")}</h3>
            <div className="fb-cond">
              {cur.when.map((c, i) => {
                const vo = valueOpts(c.field, cur);
                const needs = !["empty", "filled"].includes(c.op);
                const set = (p: Partial<Cond>) => upd({ when: cur.when.map((x, j) => (j === i ? { ...x, ...p } : x)) });
                return (
                  <div key={i}>
                    {i > 0 && <button type="button" className="fb-join" onClick={() => upd({ any: !cur.any })}>{cur.any ? t("or") : t("and")}</button>}
                    <div className="fb-rule">
                      <Dropdown options={fieldOpts} value={c.field} onChange={(v) => set({ field: v, value: undefined })} placeholder={t("Field")} />
                      <Dropdown options={OPS.map((o) => ({ ...o, label: t(o.label) }))} value={c.op} onChange={(v) => set({ op: v })} />
                      {needs ? (vo?.length && c.op !== "in" && c.op !== "contains"
                        ? <Dropdown options={vo} value={c.value} onChange={(v) => set({ value: v })} placeholder={t("Value")} />
                        : <input value={c.value ?? ""} placeholder={c.op === "in" ? "a, b, c" : t("Value")} onChange={(x) => set({ value: x.target.value })} />) : <span />}
                      <button type="button" className="fb-x" aria-label={t("Remove")} onClick={() => upd({ when: cur.when.filter((_, j) => j !== i) })}>×</button>
                    </div>
                  </div>
                );
              })}
              <button type="button" className="link" onClick={() => upd({ when: [...cur.when, { field: "caseType", op: "eq" }] })}>+ {t("Add condition")}</button>
            </div>

            <h3>{t("Give the work to")}</h3>
            <label className="fb-prop"><span>{t("Method")}</span><Dropdown options={MODES.map((m) => ({ ...m, label: t(m.label) }))} value={cur.then.mode} onChange={(v) => updThen({ mode: v })} /></label>
            {["workload", "roundrobin", "skill", "queue"].includes(cur.then.mode) && (
              <label className="fb-prop"><span>{t("Work queue")}</span><Dropdown options={(queues ?? []).map((q) => ({ value: q.key, label: q.name, hint: `${(q.members ?? []).length} ${t("people")}` }))} value={cur.then.queue} onChange={(v) => updThen({ queue: v })} placeholder={t("Choose a queue")} /></label>
            )}
            {cur.then.mode === "skill" && <label className="fb-prop"><span>{t("Skill")}</span><Dropdown allowCustom options={skills.map((x) => ({ value: x, label: x }))} value={cur.then.skill} onChange={(v) => updThen({ skill: v })} /></label>}
            {cur.then.mode === "role" && <label className="fb-prop"><span>{t("Role")}</span><Dropdown allowCustom options={roles.map((x) => ({ value: x, label: x }))} value={cur.then.role} onChange={(v) => updThen({ role: v })} /></label>}
            {cur.then.mode === "user" && <label className="fb-prop"><span>{t("Person")}</span><Dropdown options={(users ?? []).map((u) => ({ value: u.id, label: u.name || u.id, hint: u.id }))} value={cur.then.user} onChange={(v) => updThen({ user: v })} /></label>}
            <div className="ar-grid">
              {["workload", "roundrobin", "skill"].includes(cur.then.mode) && (
                <label className="fb-prop"><span>{t("Max open items per person")}</span><input inputMode="numeric" value={cur.then.maxPerPerson ?? ""} placeholder={t("No limit")} onChange={(x) => updThen({ maxPerPerson: x.target.value === "" ? undefined : Number(x.target.value) })} /></label>
              )}
              <label className="fb-prop"><span>{t("Add urgency")}</span><input inputMode="numeric" value={cur.then.urgency ?? ""} placeholder="0" onChange={(x) => updThen({ urgency: x.target.value === "" ? undefined : Number(x.target.value) })} /></label>
              <label className="fb-prop"><span>{t("Deadline (replaces the step's SLA)")}</span><Dropdown allowCustom options={[{ value: "", label: "—" }, ...DURATIONS]} value={cur.then.sla ?? ""} onChange={(v) => updThen({ sla: v || undefined })} /></label>
              <label className="fb-prop"><span>{t("Goal (warning before the deadline)")}</span><Dropdown allowCustom options={[{ value: "", label: "—" }, ...DURATIONS]} value={cur.then.goal ?? ""} onChange={(v) => updThen({ goal: v || undefined })} /></label>
            </div>
            <button className="btn-s danger mt" onClick={remove}>{t("Delete rule")}</button>
          </> : <div className="muted">{t("Select a rule or add one.")}</div>}

          <div className="ar-test">
            <h3>{t("Try it")}</h3>
            <div className="ar-grid">
              <label className="fb-prop"><span>{t("Case type")}</span><Dropdown options={(procs ?? []).map((p) => ({ value: p.key, label: p.name }))} value={test.caseType} onChange={(v) => setTest({ ...test, caseType: v, step: "" })} /></label>
              <label className="fb-prop"><span>{t("Step")}</span><Dropdown options={(steps ?? []).filter((s) => s.caseType === test.caseType).map((s) => ({ value: s.step, label: s.name }))} value={test.step} onChange={(v) => setTest({ ...test, step: v })} /></label>
            </div>
            {test.data.map((d, i) => (
              <div key={i} className="ar-kv">
                <Dropdown options={(fields ?? []).map((f) => ({ value: f.key, label: f.label }))} value={d.k} onChange={(v) => setTest({ ...test, data: test.data.map((x, j) => (j === i ? { ...x, k: v } : x)) })} placeholder={t("Case data")} />
                <input value={d.v} placeholder={t("Value")} onChange={(x) => setTest({ ...test, data: test.data.map((y, j) => (j === i ? { ...y, v: x.target.value } : y)) })} />
                <button className="fb-x" aria-label={t("Remove")} onClick={() => setTest({ ...test, data: test.data.filter((_, j) => j !== i) })}>×</button>
              </div>
            ))}
            <div className="ar-row"><button className="link" onClick={() => setTest({ ...test, data: [...test.data, { k: "", v: "" }] })}>+ {t("Case data")}</button><span className="fb-sp" />
              <button className="btn-s" disabled={!test.caseType || !test.step} onClick={runTest}>{t("Who gets it?")}</button></div>
            {pv && (
              <div className="ar-result">
                <div><span className="muted">{t("Rule")}</span> {pv.rule ?? t("none")}</div>
                <div><span className="muted">{t("Goes to")}</span> {pv.assignee ? pv.assignee : pv.queue ? `${t("queue")} ${queues?.find((q) => q.key === pv.queue)?.name ?? pv.queue}` : pv.role ? `${t("role")} ${pv.role}` : "—"}</div>
                {pv.pool && <div><span className="muted">{t("Available (open items)")}</span> {pv.pool.join(", ")}</div>}
                <div><span className="muted">{t("Urgency")}</span> {pv.urgency}{pv.sla ? ` · SLA ${pv.sla}` : ""}</div>
                {pv.note && <div className="muted small">{t(pv.note)}</div>}
              </div>
            )}
          </div>
        </section>
      </div>
    </Page>
  );
}

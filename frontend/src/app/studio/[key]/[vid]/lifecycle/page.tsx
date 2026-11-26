"use client";
// Lifecycle designer (business mode): stages as columns, steps as cards.
// Drag cards between stages; the BPMN flow, stage mapping and step screens
// are generated on save.
import { useParams } from "next/navigation";
import { useEffect, useState } from "react";
import { api, CaseModel, Form, Lifecycle, LCStage, LCStep, ProcessDef, Version } from "@/lib/api";
import { t } from "@/lib/i18n";
import { useLoad } from "@/lib/useLoad";
import Dropdown from "@/components/Dropdown";
import { Viewer } from "@/components/Bpmn";
import { Err, Field } from "@/components/ui";

const KINDS: { value: LCStep["kind"]; label: string }[] = [
  { value: "task", label: "Task (fill a form)" }, { value: "approval", label: "Approval" },
  { value: "automation", label: "Automation (call a system)" }, { value: "decision", label: "Decision table" },
  { value: "wait", label: "Wait for a message" }, { value: "timer", label: "Wait for a time" }, { value: "child", label: "Sub-case" },
];
const kindLabel = (k: string) => t(KINDS.find((x) => x.value === k)?.label ?? k);
const SLA = ["PT4H", "PT8H", "P1D", "P3D", "P5D", "P7D", "P14D", "P30D"].map((x) => ({ value: x, label: x }));
const slug = (s: string) => s.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "step";

export default function LifecyclePage() {
  const { vid } = useParams<{ key: string; vid: string }>();
  const { data: v, reload: reloadV } = useLoad(() => api.get<Version>(`/api/versions/${vid}`), [vid]);
  const { data: loaded } = useLoad(() => api.get<Lifecycle>(`/api/versions/${vid}/lifecycle`), [vid]);
  const { data: model } = useLoad(() => api.get<CaseModel>(`/api/versions/${vid}/model`), [vid]);
  const { data: forms } = useLoad(() => api.get<Form[]>("/api/forms"));
  const { data: procs } = useLoad(() => api.get<ProcessDef[]>("/api/processes"));
  const [l, setL] = useState<Lifecycle>({ stages: [] });
  const [sel, setSel] = useState<{ s: number; i: number } | null>(null);
  const [drag, setDrag] = useState<{ s: number; i: number } | null>(null);
  const [over, setOver] = useState<number | null>(null);
  const [e, setE] = useState<string>();
  const [msg, setMsg] = useState<string>();
  const [dirty, setDirty] = useState(false);
  useEffect(() => { if (loaded) setL({ ...loaded, stages: loaded.stages ?? [] }); }, [loaded]);

  const draft = v?.status === "draft" && !l.detached;
  const upd = (next: Lifecycle) => { setL(next); setDirty(true); setMsg(undefined); };
  const setStage = (si: number, p: Partial<LCStage>) => upd({ ...l, stages: l.stages.map((x, j) => (j === si ? { ...x, ...p } : x)) });
  const setStep = (p: Partial<LCStep>) => sel && upd({ ...l, stages: l.stages.map((x, j) => j !== sel.s ? x : { ...x, steps: x.steps.map((y, k) => (k === sel.i ? { ...y, ...p } : y)) }) });
  const allKeys = () => new Set(l.stages.flatMap((s) => [s.key, ...s.steps.map((x) => x.key)]));
  const uniq = (base: string) => { const ks = allKeys(); let k = base, n = 2; while (ks.has(k)) k = `${base}-${n++}`; return k; };
  const addStep = (si: number) => {
    const key = uniq("step");
    const st = l.stages[si];
    upd({ ...l, stages: l.stages.map((x, j) => (j === si ? { ...x, steps: [...x.steps, { key, name: t("New step"), kind: "task" as const }] } : x)) });
    setSel({ s: si, i: st.steps.length });
  };
  const addStage = (kind: LCStage["kind"]) => {
    const key = uniq(kind === "primary" ? "stage" : kind);
    const stages = l.stages.slice();
    const res = stages.findIndex((x) => x.kind !== "primary");
    const st: LCStage = { key, name: kind === "alternate" ? t("Alternate stage") : t("New stage"), kind, steps: [] };
    if (kind === "primary" && res >= 0) stages.splice(res, 0, st); else stages.push(st);
    upd({ ...l, stages });
  };
  const dropOn = (si: number, idx?: number) => {
    if (!drag) return;
    const stages = l.stages.map((x) => ({ ...x, steps: x.steps.slice() }));
    const [step] = stages[drag.s].steps.splice(drag.i, 1);
    const at = idx ?? stages[si].steps.length;
    stages[si].steps.splice(drag.s === si && drag.i < at ? at - 1 : at, 0, step);
    upd({ ...l, stages });
    setDrag(null); setOver(null); setSel(null);
  };
  const shift = (d: number) => {
    if (!sel) return;
    const stages = l.stages.map((x) => ({ ...x, steps: x.steps.slice() }));
    const steps = stages[sel.s].steps;
    const j = sel.i + d;
    [steps[sel.i], steps[j]] = [steps[j], steps[sel.i]];
    upd({ ...l, stages });
    setSel({ s: sel.s, i: j });
  };
  const moveTo = (to: number) => {
    if (!sel || to === sel.s) return;
    const stages = l.stages.map((x) => ({ ...x, steps: x.steps.slice() }));
    const [st] = stages[sel.s].steps.splice(sel.i, 1);
    stages[to].steps.push(st);
    upd({ ...l, stages });
    setSel({ s: to, i: stages[to].steps.length - 1 });
    setDrag(null);
  };
  const save = async () => {
    try {
      const r = await api.put<{ version: Version; warnings: string[] }>(`/api/versions/${vid}/lifecycle`, l);
      setMsg(t("Saved") + " · " + t("flow regenerated") + (r.warnings?.length ? ` · ${r.warnings.join("; ")}` : ""));
      setE(undefined); setDirty(false); reloadV();
    } catch (x: any) { setE(x.message); }
  };

  const step = sel ? l.stages[sel.s]?.steps[sel.i] : undefined;
  const allSteps = l.stages.flatMap((x) => x.steps);
  const human = allSteps.filter((x) => x.kind === "task" || x.kind === "approval");
  const readiness = [
    { ok: l.stages.some((x) => x.kind === "primary" && x.steps.length > 0), text: t("The case has at least one stage with steps") },
    { ok: human.every((x) => !!x.role), text: human.filter((x) => !x.role).length ? `${t("Steps without someone responsible")}: ${human.filter((x) => !x.role).map((x) => x.name).join(", ")}` : t("Every step has someone responsible") },
    { ok: human.every((x) => !!x.form), text: human.filter((x) => !x.form).length ? `${t("Steps without a form")}: ${human.filter((x) => !x.form).map((x) => x.name).join(", ")}` : t("Every task has a form") },
    { ok: allSteps.filter((x) => x.kind === "automation").every((x) => !!x.connector), text: t("Every automation calls an integration") },
    { ok: allSteps.filter((x) => x.kind === "decision").every((x) => !!x.decision), text: t("Every decision uses a rule table") },
    { ok: !dirty, text: dirty ? t("There are unsaved changes") : t("All changes are saved") },
  ];
  const fieldOpts = (model?.data ?? []).map((f) => ({ value: f.key, label: f.label }));
  const targets = [{ value: "end", label: t("End the case (rejected)") }, ...l.stages.filter((x) => x.kind !== "resolution").map((x) => ({ value: x.key, label: `${t("Stage")}: ${x.name}` })),
    ...l.stages.flatMap((x) => x.steps).filter((x) => x.key !== step?.key).map((x) => ({ value: x.key, label: `${t("Step")}: ${x.name}` }))];

  return (
    <div>
      <div className="page-head">
        <h1>{t("Lifecycle")}</h1>
        <div className="actions">
          {draft && <><button className="btn-o" onClick={() => addStage("primary")}>{t("Add stage")}</button>
            <button className="btn-o" onClick={() => addStage("alternate")}>{t("Add alternate stage")}</button>
            <button className="btn" disabled={!dirty} onClick={save}>{t("Save")}</button></>}
        </div>
      </div>
      <Err e={e} />{msg && <div className="ok">{msg}</div>}
      {l.detached && <div className="warn">{t("The flow was edited in developer mode. Edit it in BPMN flow.")}</div>}
      {v && v.status !== "draft" && <div className="warn">{t("This version cannot be edited. Copy it to a new version.")}</div>}

      <div className="studio-split">
        <div className="board">
          {l.stages.map((st, si) => (
            <div key={st.key} className={`lane lane-${st.kind}`}>
              <div className="lane-head">
                <input value={st.name} disabled={!draft} onChange={(x) => setStage(si, { name: x.target.value })} aria-label={t("Stage name")} />
                <div className="small muted">{t(st.kind === "primary" ? "Primary" : st.kind === "alternate" ? "Alternate" : "Resolution")}
                  {st.sla ? ` · SLA ${st.sla}` : ""}</div>
                {draft && st.kind !== "resolution" && (
                  <div className="row2">
                    <Dropdown options={SLA} value={st.sla} allowCustom onChange={(x) => setStage(si, { sla: x })} placeholder={t("Stage SLA")} />
                    <button className="link danger" onClick={() => { upd({ ...l, stages: l.stages.filter((_, j) => j !== si) }); setSel(null); }}>{t("Remove")}</button>
                  </div>
                )}
              </div>
              {st.kind !== "resolution" && (
                <div className={"lane-steps" + (over === si ? " over" : "")}
                  onDragOver={(x) => { if (drag) { x.preventDefault(); setOver(si); } }}
                  onDragLeave={() => setOver(null)} onDrop={() => dropOn(si)}>
                  {st.steps.map((sp, i) => (
                    <div key={sp.key} className={"card-step" + (sel?.s === si && sel.i === i ? " on" : "")} draggable={draft}
                      onDragStart={() => setDrag({ s: si, i })} onDragEnd={() => { setDrag(null); setOver(null); }}
                      onDrop={(x) => { x.stopPropagation(); dropOn(si, i); }} onClick={() => setSel({ s: si, i })}>
                      <span className="kind">{kindLabel(sp.kind)}</span>
                      <span>{sp.name}</span>
                      <span className="small muted">{[sp.role, sp.sla && `SLA ${sp.sla}`, sp.when && t("conditional")].filter(Boolean).join(" · ")}</span>
                    </div>
                  ))}
                  {draft && <button className="add-step" onClick={() => addStep(si)}>+ {t("Add step")}</button>}
                </div>
              )}
              {st.kind === "resolution" && <div className="lane-steps small muted">{t("Resolution")}</div>}
            </div>
          ))}
        </div>

        <aside className="panel">
          {step ? (
            <>
              <h3>{t("Step")}</h3>
              <Field label={t("Name")}><input value={step.name} disabled={!draft} onChange={(x) => setStep({ name: x.target.value })} /></Field>
              <Field label={t("Type")}><Dropdown options={KINDS.map((k) => ({ value: k.value, label: t(k.label) }))} value={step.kind} disabled={!draft} onChange={(k) => setStep({ kind: k })} /></Field>
              {["task", "approval"].includes(step.kind) && <>
                <Field label={t("Role")}><input value={step.role ?? ""} disabled={!draft} onChange={(x) => setStep({ role: x.target.value })} /></Field>
                <Field label={t("Form")}><Dropdown options={(forms ?? []).map((f) => ({ value: f.key, label: f.name }))} value={step.form} disabled={!draft} onChange={(x) => setStep({ form: x })} placeholder="—" /></Field>
                <Field label="SLA"><Dropdown options={SLA} value={step.sla} allowCustom disabled={!draft} onChange={(x) => setStep({ sla: x })} placeholder="—" /></Field>
                <Field label={t("One task per item of")}><Dropdown options={fieldOpts} value={step.multi} disabled={!draft} onChange={(x) => setStep({ multi: x })} placeholder="—" /></Field>
              </>}
              {step.kind === "approval" && <>
                <Field label={t("When rejected, go to")}><Dropdown options={targets} value={step.onReject ?? "end"} disabled={!draft} onChange={(x) => setStep({ onReject: x })} /></Field>
                <Field label={t("Decision field")} hint={`${step.key.replace(/-/g, "_")}_approved`}>
                  <Dropdown options={fieldOpts} value={step.decisionField} disabled={!draft} onChange={(x) => setStep({ decisionField: x })} placeholder="—" /></Field>
              </>}
              {step.kind === "automation" && <Field label={t("Integration")}><Dropdown options={(model?.connectors ?? []).map((c) => ({ value: c.key, label: c.name }))} value={step.connector} disabled={!draft} onChange={(x) => setStep({ connector: x })} /></Field>}
              {step.kind === "decision" && <Field label={t("Decision table")}><Dropdown options={(model?.decisions ?? []).map((d) => ({ value: d.key, label: d.name }))} value={step.decision} disabled={!draft} onChange={(x) => setStep({ decision: x })} /></Field>}
              {step.kind === "wait" && <Field label={t("Message name")}><input value={step.message ?? ""} disabled={!draft} onChange={(x) => setStep({ message: x.target.value })} /></Field>}
              {step.kind === "timer" && <Field label={t("Wait")}><Dropdown options={SLA} value={step.sla} allowCustom disabled={!draft} onChange={(x) => setStep({ sla: x })} /></Field>}
              {step.kind === "child" && <Field label={t("Case type")}><Dropdown options={(procs ?? []).map((p) => ({ value: p.key, label: p.name }))} value={step.caseType} disabled={!draft} onChange={(x) => setStep({ caseType: x })} /></Field>}
              <Field label={t("Only when")} hint={t("e.g. ") + 'amount > 100000'}><input className="mono" value={step.when ?? ""} disabled={!draft} onChange={(x) => setStep({ when: x.target.value || undefined })} /></Field>
              <div className="dev-only"><Field label={t("Key")}><input className="mono" value={step.key} disabled={!draft} onChange={(x) => setStep({ key: slug(x.target.value) })} /></Field></div>
              {draft && <>
                <Field label={t("Stage")}><Dropdown options={l.stages.filter((x) => x.kind !== "resolution").map((x, j) => ({ value: String(j), label: x.name }))} value={String(sel!.s)}
                  onChange={(j) => moveTo(Number(j))} /></Field>
                <div className="bar" style={{ marginTop: 0, marginBottom: 16 }}>
                  <button className="btn-s" disabled={sel!.i === 0} onClick={() => shift(-1)}>{t("Up")}</button>
                  <button className="btn-s" disabled={sel!.i >= l.stages[sel!.s].steps.length - 1} onClick={() => shift(1)}>{t("Down")}</button>
                </div>
              </>}
              {draft && <button className="btn-s danger" onClick={() => { upd({ ...l, stages: l.stages.map((x, j) => j !== sel!.s ? x : { ...x, steps: x.steps.filter((_, k) => k !== sel!.i) }) }); setSel(null); }}>{t("Remove step")}</button>}
            </>
          ) : (
            <>
              <h3>{t("Before submitting")}</h3>
              <ul className="checklist">
                {readiness.map((r, i) => <li key={i} className={r.ok ? "" : "todo"}>{r.text}</li>)}
              </ul>
            </>
          )}
        </aside>
      </div>

      {v && <section className="card mt"><h3>{t("Generated flow")}</h3><Viewer xml={v.bpmn} active={step ? [step.key.replace(/-/g, "_")] : []} /></section>}
    </div>
  );
}

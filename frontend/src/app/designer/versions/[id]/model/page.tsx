"use client";
// Case model designer (business mode): stages and which flow steps they
// contain, the case data model with calculated fields and rules, and the
// actions users may run outside the flow.
import Link from "next/link";
import { useParams } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { api, ActionDef, CaseModel, DataFieldDef, Form, ProcessDef, StageDef, Version } from "@/lib/api";
import { useLoad } from "@/lib/useLoad";
import { t } from "@/lib/i18n";
import Dropdown from "@/components/Dropdown";
import { Viewer } from "@/components/Bpmn";
import { Err, Page, Status } from "@/components/ui";

const KINDS = [{ value: "primary", label: "Primary" }, { value: "alternate", label: "Alternate" }, { value: "resolution", label: "Resolution" }];
const TYPES = ["text", "longtext", "number", "money", "date", "boolean", "enum", "list", "user", "org", "reference"].map((x) => ({ value: x, label: x }));
const ACTION_KINDS = [{ value: "update", label: "Update case data" }, { value: "stage", label: "Go to stage" }, { value: "child", label: "Open child case" }];
const SLA = ["PT8H", "P1D", "P3D", "P5D", "P7D", "P14D", "P30D", "P60D"].map((x) => ({ value: x, label: x }));
const slug = (s: string) => s.toLowerCase().normalize("NFKD").replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "item";
const ident = (s: string) => (s.replace(/[^A-Za-z0-9]+(.)?/g, (_, c) => (c ? c.toUpperCase() : "")).replace(/^[^A-Za-z_]+/, "") || "field").replace(/^./, (c) => c.toLowerCase());

type Tab = "stages" | "data" | "actions";

export default function ModelDesigner() {
  const prm = useParams<{ id?: string; vid?: string }>();
  const id = (prm.id ?? prm.vid) as string; // also mounted inside the App Studio
  const { data: v } = useLoad(() => api.get<Version>(`/api/versions/${id}`), [id]);
  const { data: loaded, error } = useLoad(() => api.get<CaseModel>(`/api/versions/${id}/model`), [id]);
  const { data: forms } = useLoad(() => api.get<Form[]>("/api/forms"));
  const { data: procs } = useLoad(() => api.get<ProcessDef[]>("/api/processes"));
  const [m, setM] = useState<CaseModel>({ stages: [], data: [], rules: [], actions: [] });
  const inStudio = !!prm.vid;
  const [tab, setTab] = useState<Tab>(() => {
    if (typeof window === "undefined") return "stages";
    const seg = location.pathname.split("/")[4];
    const q = new URLSearchParams(location.search).get("tab") ?? seg;
    return (q === "data" || q === "actions" ? q : "stages") as Tab;
  });
  const [e, setE] = useState<string>();
  const [msg, setMsg] = useState<string>();
  const [focus, setFocus] = useState<string>();
  useEffect(() => { if (loaded) setM({ rules: [], ...loaded }); }, [loaded]);

  // flow steps from the BPMN (activities, events, gateways)
  const steps = useMemo(() => {
    if (!v) return [] as { id: string; name: string; type: string }[];
    const out: { id: string; name: string; type: string }[] = [];
    const re = /<(\w+)\s([^>]*?)\/?>/g;
    let x: RegExpExecArray | null;
    while ((x = re.exec(v.bpmn))) {
      const tag = x[1];
      if (!/(Task|task|Event|Gateway|subProcess|callActivity)$/.test(tag) || tag.endsWith("Definition")) continue;
      const idm = /\bid="([^"]+)"/.exec(x[2]);
      const nm = /\bname="([^"]*)"/.exec(x[2]);
      if (idm) out.push({ id: idm[1], name: nm?.[1] || idm[1], type: tag });
    }
    return out;
  }, [v]);

  const draft = v?.status === "draft";
  const stageOpts = m.stages.map((s) => ({ value: s.key, label: s.name }));
  const fieldOpts = m.data.map((f) => ({ value: f.key, label: f.label }));
  const stepOpts = steps.map((s) => ({ value: s.id, label: s.name, hint: s.type }));
  const ownerOf = (step: string) => m.stages.find((s) => s.steps.includes(step))?.key ?? "";

  const setStage = (i: number, p: Partial<StageDef>) => setM({ ...m, stages: m.stages.map((s, j) => (j === i ? { ...s, ...p } : s)) });
  const setField = (i: number, p: Partial<DataFieldDef>) => setM({ ...m, data: m.data.map((s, j) => (j === i ? { ...s, ...p } : s)) });
  const setAction = (i: number, p: Partial<ActionDef>) => setM({ ...m, actions: m.actions.map((s, j) => (j === i ? { ...s, ...p } : s)) });
  const assign = (step: string, stage: string) =>
    setM({ ...m, stages: m.stages.map((s) => ({ ...s, steps: s.key === stage ? [...s.steps.filter((x) => x !== step), step] : s.steps.filter((x) => x !== step) })) });
  const move = <T,>(list: T[], i: number, d: number) => {
    const j = i + d;
    if (j < 0 || j >= list.length) return list;
    const n = list.slice(); [n[i], n[j]] = [n[j], n[i]]; return n;
  };

  const save = () => api.put<CaseModel>(`/api/versions/${id}/model`, m)
    .then((x) => { setM({ rules: [], ...x }); setMsg(t("Saved")); setE(undefined); })
    .catch((x) => { setE(x.message); setMsg(undefined); });

  const unassigned = steps.filter((s) => !ownerOf(s.id) && !/Event$/.test(s.type) && !/Gateway$/.test(s.type));

  return (
    <Page title={inStudio ? t(tab === "data" ? "Data model" : tab === "actions" ? "Actions" : "Stages") : `${t("Case model")} · ${v?.processKey ?? ""} v${v?.no ?? ""}`} actions={inStudio ? (draft ? <button className="btn" onClick={save}>{t("Save")}</button> : null) : <>
      {v && <Status s={v.status} />}
      <Link className="btn-o" href={`/designer/versions/${id}`}>{t("Edit flow")}</Link>
      <Link className="btn-o" href={`/designer/versions/${id}/mapping`}>{t("Step screens")}</Link>
      {draft && <button className="btn" onClick={save}>{t("Save")}</button>}
    </>}>
      <Err e={error ?? e} />{msg && <div className="ok">{msg}</div>}
      {v && !draft && <div className="warn">{t("This version cannot be edited. Copy it to a new version.")}</div>}
      {!inStudio && <div className="tabs">
        {(["stages", "data", "actions"] as Tab[]).map((x) => (
          <button key={x} className={tab === x ? "on" : ""} onClick={() => setTab(x)}>
            {x === "stages" ? `${t("Stages")} (${m.stages.length})` : x === "data" ? `${t("Data model")} (${m.data.length})` : `${t("Actions")} (${m.actions.length})`}
          </button>
        ))}
      </div>}

      {tab === "stages" && (
        <>
          {v && <Viewer xml={v.bpmn} active={focus ? (m.stages.find((s) => s.key === focus)?.steps ?? [focus]) : []} />}
          <section className="card mt">
            <h3>{t("Stages")}</h3>
            <table className="tbl map">
              <thead><tr><th>{t("Name")}</th><th className="dev-only">{t("Key")}</th><th>{t("Type")}</th><th>{t("Steps")}</th><th>{t("Entry step")}</th><th>SLA</th><th /></tr></thead>
              <tbody>
                {m.stages.map((s, i) => (
                  <tr key={i} onMouseEnter={() => setFocus(s.key)}>
                    <td><input value={s.name} disabled={!draft} onChange={(x) => setStage(i, { name: x.target.value, ...(s.key ? {} : { key: slug(x.target.value) }) })} /></td>
                    <td className="dev-only"><input value={s.key} disabled={!draft} onChange={(x) => setStage(i, { key: slug(x.target.value) })} /></td>
                    <td><Dropdown options={KINDS} value={s.kind} disabled={!draft} onChange={(k) => setStage(i, { kind: k })} /></td>
                    <td className="small">{s.steps.map((n) => <span key={n} className="chip">{steps.find((x) => x.id === n)?.name ?? n}</span>)}</td>
                    <td><Dropdown options={stepOpts} value={s.entry} disabled={!draft} onChange={(n) => setStage(i, { entry: n })} placeholder="—" /></td>
                    <td><Dropdown options={SLA} value={s.sla} allowCustom disabled={!draft} onChange={(x) => setStage(i, { sla: x })} placeholder="—" /></td>
                    <td className="nw">{draft && <>
                      <button className="link" onClick={() => setM({ ...m, stages: move(m.stages, i, -1) })}>{t("Up")}</button>
                      <button className="link danger" onClick={() => setM({ ...m, stages: m.stages.filter((_, j) => j !== i) })}>{t("Remove")}</button>
                    </>}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            {draft && <div className="bar"><button className="btn-s" onClick={() => setM({ ...m, stages: [...m.stages, { key: `stage-${m.stages.length + 1}`, name: `Stage ${m.stages.length + 1}`, kind: "primary", steps: [] }] })}>{t("Add stage")}</button></div>}
          </section>
          <section className="card mt">
            <h3>{t("Steps by stage")}</h3>
            {unassigned.length > 0 && <div className="warn">{t("Steps without a stage")}: {unassigned.map((s) => s.name).join(", ")}</div>}
            <table className="tbl">
              <thead><tr><th>{t("Step")}</th><th>{t("Type")}</th><th>{t("Stage")}</th></tr></thead>
              <tbody>
                {steps.map((s) => (
                  <tr key={s.id} onMouseEnter={() => setFocus(s.id)}>
                    <td>{s.name}<div className="muted small dev-only">{s.id}</div></td>
                    <td className="small">{s.type}</td>
                    <td><Dropdown options={stageOpts} value={ownerOf(s.id)} disabled={!draft} onChange={(k) => assign(s.id, k)} placeholder="—" /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>
        </>
      )}

      {tab === "data" && (
        <>
          <section className="card">
            <h3>{t("Fields")}</h3>
            <table className="tbl map">
              <thead><tr><th>{t("Label")}</th><th className="dev-only">{t("Key")}</th><th>{t("Type")}</th><th>{t("Required")}</th><th>{t("Required in")}</th><th>{t("Options")} / {t("Calculation")}</th><th>{t("Summary")}</th><th /></tr></thead>
              <tbody>
                {m.data.map((f, i) => (
                  <tr key={i}>
                    <td><input value={f.label} disabled={!draft} onChange={(x) => setField(i, { label: x.target.value, ...(f.key ? {} : { key: ident(x.target.value) }) })} /></td>
                    <td className="dev-only"><input className="mono" value={f.key} disabled={!draft} onChange={(x) => setField(i, { key: ident(x.target.value) })} /></td>
                    <td><Dropdown options={TYPES} value={f.type} disabled={!draft} onChange={(x) => setField(i, { type: x })} /></td>
                    <td><button type="button" disabled={!draft || !!f.calc} className={"check" + (f.required ? " on" : "")} onClick={() => setField(i, { required: !f.required })}>{f.required ? "✓" : ""}</button></td>
                    <td><Dropdown multiple options={stageOpts} value={f.requiredIn ?? []} disabled={!draft || !f.required} onChange={(x) => setField(i, { requiredIn: x })} placeholder={t("Any stage")} /></td>
                    <td>
                      {f.type === "enum"
                        ? <input placeholder="value=Label, value=Label" disabled={!draft} value={(f.options ?? []).map((o) => `${o.value}=${o.label}`).join(", ")}
                            onChange={(x) => setField(i, { options: x.target.value.split(",").map((p) => p.trim()).filter(Boolean).map((p) => { const [a, ...b] = p.split("="); return { value: a.trim(), label: (b.join("=") || a).trim() }; }) })} />
                        : <input className="mono" placeholder={t("e.g. ") + "amount * 0.07"} disabled={!draft} value={f.calc ?? ""} onChange={(x) => setField(i, { calc: x.target.value || undefined, ...(x.target.value ? { required: false } : {}) })} />}
                    </td>
                    <td><button type="button" disabled={!draft} className={"check" + (f.summary ? " on" : "")} onClick={() => setField(i, { summary: !f.summary })}>{f.summary ? "✓" : ""}</button></td>
                    <td className="nw">{draft && <>
                      <button className="link" onClick={() => setM({ ...m, data: move(m.data, i, -1) })}>{t("Up")}</button>
                      <button className="link danger" onClick={() => setM({ ...m, data: m.data.filter((_, j) => j !== i) })}>{t("Remove")}</button>
                    </>}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            {draft && <div className="bar"><button className="btn-s" onClick={() => setM({ ...m, data: [...m.data, { key: "", label: "", type: "text" }] })}>{t("Add field")}</button></div>}
          </section>
          <section className="card mt">
            <h3>{t("Validation rules")}</h3>
            <table className="tbl map">
              <thead><tr><th>{t("Applies when")}</th><th>{t("Must be true")}</th><th>{t("Message")}</th><th /></tr></thead>
              <tbody>
                {(m.rules ?? []).map((r, i) => (
                  <tr key={i}>
                    <td><input className="mono" disabled={!draft} placeholder={t("always")} value={r.when ?? ""} onChange={(x) => setM({ ...m, rules: m.rules!.map((y, j) => (j === i ? { ...y, when: x.target.value || undefined } : y)) })} /></td>
                    <td><input className="mono" disabled={!draft} value={r.check} onChange={(x) => setM({ ...m, rules: m.rules!.map((y, j) => (j === i ? { ...y, check: x.target.value } : y)) })} /></td>
                    <td><input disabled={!draft} value={r.message} onChange={(x) => setM({ ...m, rules: m.rules!.map((y, j) => (j === i ? { ...y, message: x.target.value } : y)) })} /></td>
                    <td>{draft && <button className="link danger" onClick={() => setM({ ...m, rules: m.rules!.filter((_, j) => j !== i) })}>{t("Remove")}</button>}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            {draft && <div className="bar"><button className="btn-s" onClick={() => setM({ ...m, rules: [...(m.rules ?? []), { check: "", message: "" }] })}>{t("Add rule")}</button></div>}
          </section>
        </>
      )}

      {tab === "actions" && (
        <section className="card">
          <h3>{t("Actions")}</h3>
          <table className="tbl map">
            <thead><tr><th>{t("Label")}</th><th>{t("Type")}</th><th>{t("Target")}</th><th>{t("Form")}</th><th>{t("Available in")}</th><th>{t("Roles")}</th><th>{t("Copy fields")}</th><th /></tr></thead>
            <tbody>
              {m.actions.map((a, i) => (
                <tr key={i}>
                  <td><input value={a.label} disabled={!draft} onChange={(x) => setAction(i, { label: x.target.value, ...(a.key ? {} : { key: slug(x.target.value) }) })} /><div className="muted small mono dev-only">{a.key}</div></td>
                  <td><Dropdown options={ACTION_KINDS} value={a.kind} disabled={!draft} onChange={(k) => setAction(i, { kind: k, target: undefined })} /></td>
                  <td>
                    {a.kind === "stage" && <Dropdown options={stageOpts} value={a.target} disabled={!draft} onChange={(x) => setAction(i, { target: x })} />}
                    {a.kind === "child" && <Dropdown options={(procs ?? []).map((p) => ({ value: p.key, label: p.name }))} value={a.target} disabled={!draft} onChange={(x) => setAction(i, { target: x })} />}
                  </td>
                  <td><Dropdown options={(forms ?? []).map((f) => ({ value: f.key, label: f.name }))} value={a.formKey} disabled={!draft} onChange={(x) => setAction(i, { formKey: x })} placeholder="—" /></td>
                  <td><Dropdown multiple options={stageOpts} value={a.stages ?? []} disabled={!draft} onChange={(x) => setAction(i, { stages: x })} placeholder={t("Any stage")} /></td>
                  <td><Dropdown multiple allowCustom options={(a.roles ?? []).map((r) => ({ value: r, label: r }))} value={a.roles ?? []} disabled={!draft} onChange={(x) => setAction(i, { roles: x })} placeholder={t("All roles")} /></td>
                  <td>{a.kind === "child" && <Dropdown multiple options={fieldOpts} value={a.pass ?? []} disabled={!draft} onChange={(x) => setAction(i, { pass: x })} placeholder="—" />}</td>
                  <td>{draft && <button className="link danger" onClick={() => setM({ ...m, actions: m.actions.filter((_, j) => j !== i) })}>{t("Remove")}</button>}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {draft && <div className="bar"><button className="btn-s" onClick={() => setM({ ...m, actions: [...m.actions, { key: "", label: "", kind: "update" }] })}>{t("Add action")}</button></div>}
        </section>
      )}
    </Page>
  );
}

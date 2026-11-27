"use client";
// Case view: stage bar, summary, actions, and tabs for work, data, related
// cases, timeline and flow. Supervisors can move steps and cancel.
import Link from "next/link";
import { useParams } from "next/navigation";
import { useEffect, useState } from "react";
import { api, ActionView, Audit, Case, CaseModel, EventRec, Form, StageView, Version, Work } from "@/lib/api";
import { dt, fieldValue, loopLabel, money, overdue } from "@/lib/format";
import { t } from "@/lib/i18n";
import { useDialog } from "@/components/useDialog";
import { useLoad } from "@/lib/useLoad";
import { has, useSession } from "@/lib/useSession";
import Dropdown from "@/components/Dropdown";
import FormRenderer, { validate } from "@/components/FormRenderer";
import Modal from "@/components/Modal";
import StageBar from "@/components/StageBar";
import InlineTask from "@/components/InlineTask";
import { Viewer } from "@/components/Bpmn";
import { auditText } from "@/components/ScreenRenderer";
import { Err, Page, Status } from "@/components/ui";

type Incident = { id: string; nodeName: string; message: string; status: string };
type CaseView = {
  case: Case; work: Work[]; audit: Audit[]; version: Version & { model?: CaseModel }; children: Case[]; incidents?: Incident[];
  stages?: StageView[]; actions?: ActionView[]; mine?: string[]; missing?: string[]; related?: Case[]; parent?: Case; events?: EventRec[];
};
type Tab = "work" | "data" | "related" | "timeline" | "flow";

const fmt = (v: unknown, f?: Parameters<typeof fieldValue>[1] | string) => fieldValue(v, typeof f === "string" ? { type: f } : f);

export default function CasePage() {
  const { id } = useParams<{ id: string }>();
  const s = useSession();
  // Go encodes empty slices as null: normalise once here
  const { data, error, reload } = useLoad(() => api.get<CaseView>(`/api/cases/${id}`).then((d) => ({
    ...d, work: d.work ?? [], audit: d.audit ?? [], children: d.children ?? [], incidents: d.incidents ?? [],
    stages: d.stages ?? [], actions: d.actions ?? [], missing: d.missing ?? [], related: d.related ?? [], events: d.events ?? [],
    case: { ...d.case, instance: { ...d.case.instance, tokens: d.case.instance.tokens ?? [], history: d.case.instance.history ?? [] } },
  })), [id]);
  const [tab, setTab] = useState<Tab>("work");
  const dlg = useDialog();
  const [e, setE] = useState<string>();
  const [msg, setMsg] = useState<string>();
  const [moveTok, setMoveTok] = useState("");
  const [moveTo, setMoveTo] = useState("");
  const [reason, setReason] = useState("");
  const [action, setAction] = useState<ActionView>();
  const [aform, setAform] = useState<Form>();
  const [adata, setAdata] = useState<Record<string, any>>({});
  const [aerr, setAerr] = useState<Record<string, string>>({});
  const [relate, setRelate] = useState("");
  useEffect(() => {
    setAform(undefined); setAdata({}); setAerr({});
    if (action?.formKey) api.get<Form>(`/api/forms/${action.formKey}`).then(setAform).catch(() => {});
  }, [action]);

  if (!data) return <Page title={t("Case")}><Err e={error} /></Page>;
  const c = data.case;
  const model = data.version.model;
  const sup = has(s, "cm:supervisor");
  const running = c.status === "running";
  const nodes = Array.from(data.version.bpmn.matchAll(/<(userTask|manualTask|task)\s[^>]*id="([^"]+)"[^>]*name="([^"]*)"/g)).map((m) => ({ value: m[2], label: m[3] || m[2] }));
  const act = (p: Promise<unknown>, ok?: string) => p.then(() => { reload(); setE(undefined); if (ok) setMsg(ok); }).catch((x) => setE(x.message));
  const open = data.work.filter((w) => w.status === "open" || w.status === "claimed");
  // what the viewer can do right here: assigned to them, or open in one of their queues
  const mineNow = open.filter((w) => (data.mine ?? []).includes(w.id));
  const stageName = (k?: string) => model?.stages.find((x) => x.key === k)?.name ?? k;
  const summary = (model?.data ?? []).filter((f) => f.summary);

  const runAction = async () => {
    if (!action) return;
    const fields = aform?.schema.fields ?? [];
    const v = validate(fields, adata, aform?.schema.pages);
    setAerr(v);
    if (Object.keys(v).length) return;
    try {
      const r = await api.post<Case>(`/api/cases/${c.id}/actions/${action.key}`, adata);
      setAction(undefined);
      setMsg(action.kind === "child" ? `${t("Child case opened")}: ${r.number}` : `${action.label}: ${t("done")}`);
      setE(undefined);
      reload();
    } catch (x: any) { setAerr({ _: x.message }); }
  };

  // timeline: stage changes + audit entries, newest first
  const timeline = [
    ...(data.events ?? []).filter((x) => x.type === "stage").map((x) => ({ at: x.at, by: x.by ?? "", text: `${t("Entered stage")} ${stageName(x.node)}`, stage: true })),
    ...data.audit.map((a) => ({ at: a.at, by: a.actor, text: auditText(a), stage: false })),
  ].sort((a, b) => b.at.localeCompare(a.at));

  return (
    <Page title={`${c.number} · ${c.title}`} actions={<>
      <Status s={c.status} />
      {(data.actions ?? []).filter((a) => a.allowed).map((a) => (
        <button key={a.key} className="btn-o" onClick={() => setAction(a)}>{a.label}</button>
      ))}
    </>}>
      {dlg.node}
      <Err e={e ?? c.instance.error} />{msg && <div className="ok">{msg}</div>}
      {(data.incidents ?? []).filter((i) => i.status === "open").map((i) => (
        <div key={i.id} className="err">{t("Incident at")} {i.nodeName}: {i.message} · <Link href="/incidents">{t("Resolve")}</Link></div>
      ))}
      {(data.missing ?? []).length > 0 && running && <div className="warn">{t("Missing data")}: {data.missing!.join(", ")}</div>}

      <StageBar stages={data.stages ?? []} />
      <div className="case-head">
        <div className="kv1"><span>{t("Stage")}</span><span>{stageName(c.stage) ?? "-"}</span></div>
        {summary.map((f) => <div key={f.key} className="kv1"><span>{f.label}</span><span>{fmt(c.instance.vars[f.key], f)}</span></div>)}
        <div className="kv1"><span>{t("Initiator")}</span><span>{c.createdBy}</span></div>
        <div className="kv1"><span>{t("Created")}</span><span>{dt(c.createdAt)}</span></div>
        {data.parent && <div className="kv1"><span>{t("Parent case")}</span><Link href={`/cases/${data.parent.id}`}>{data.parent.number}</Link></div>}
      </div>

      {running && mineNow.map((w) => <InlineTask key={w.id} work={w} me={s?.id ?? ""} onDone={() => { setMsg(`${w.name}: ${t("done")}`); reload(); }} />)}

      <div className="tabs">
        {([["work", `${t("Open work")} (${open.length})`], ["data", t("Data")], ["related", `${t("Related")} (${data.children.length + (data.related?.length ?? 0)})`],
          ["timeline", t("Timeline")], ["flow", t("Flow diagram")]] as [Tab, string][]).map(([k, label]) => (
          <button key={k} className={tab === k ? "on" : ""} onClick={() => setTab(k)}>{label}</button>
        ))}
      </div>

      {tab === "work" && (
        <section className="card">
          {open.length ? (
            <table className="tbl">
              <thead><tr><th>{t("Step")}</th><th>{t("Stage")}</th><th>{t("Role")}</th><th>{t("Assignee")}</th><th>{t("Due")}</th><th /></tr></thead>
              <tbody>
                {open.map((w) => (
                  <tr key={w.id} className={overdue(w.dueAt) ? "late" : ""}>
                    <td><Link href={`/work/${w.id}`}>{w.name}</Link>{loopLabel(w.input)}</td>
                    <td>{stageName(model?.stages.find((x) => x.steps.includes(w.node))?.key) ?? "-"}</td>
                    <td>{w.queue ? `${t("Queue")} ${w.queue}` : w.role}</td><td>{w.assignee || "-"}</td><td className="nw">{dt(w.dueAt)}</td>
                    <td className="nw">{sup && <button className="link" onClick={() => { setMoveTok(w.tokenId); setMoveTo(""); }}>{t("Move step")}</button>}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : <div className="muted">-</div>}
          {sup && moveTok && (
            <div className="row3 mt">
              <Dropdown placeholder={t("Target step")} value={moveTo} onChange={setMoveTo} options={nodes} />
              <input placeholder={t("Reason")} value={reason} onChange={(x) => setReason(x.target.value)} />
              <div>
                <button className="btn" disabled={!moveTo} onClick={() => act(api.post(`/api/cases/${c.id}/move`, { token: moveTok, to: moveTo, reason })).then(() => setMoveTok(""))}>{t("Move")}</button>
                <button className="link" onClick={() => setMoveTok("")}>{t("Cancel")}</button>
              </div>
            </div>
          )}
          {sup && running && (
            <div className="bar"><button className="btn-o danger" onClick={async () => {
              const why = await dlg.prompt(t("Cancel case"), t("Reason for cancelling"), "", { ok: t("Cancel case"), danger: true });
              if (why) act(api.post(`/api/cases/${c.id}/cancel`, { reason: why }));
            }}>{t("Cancel case")}</button></div>
          )}
        </section>
      )}

      {tab === "data" && (
        <section className="card">
          <dl className="kv">
            {(model?.data.length ? model.data.map((f) => [f.label + (f.calc ? " ƒ" : ""), fmt(c.instance.vars[f.key], f)] as const)
              : Object.entries(c.instance.vars).filter(([, v]) => typeof v !== "object" || Array.isArray(v)).map(([k, v]) => [k, fmt(v)] as const))
              .map(([k, v]) => <span key={k} style={{ display: "contents" }}><dt>{k}</dt><dd>{v}</dd></span>)}
          </dl>
        </section>
      )}

      {tab === "related" && (
        <section className="card">
          <h3>{t("Sub-cases")}</h3>
          {data.children.length ? (
            <table className="tbl"><thead><tr><th>{t("No.")}</th><th>{t("Title")}</th><th>{t("Stage")}</th><th>{t("Status")}</th></tr></thead>
              <tbody>{data.children.map((k) => <tr key={k.id}><td><Link href={`/cases/${k.id}`}>{k.number}</Link></td><td>{k.title}</td><td>{k.stage ?? "-"}</td><td><Status s={k.status} /></td></tr>)}</tbody></table>
          ) : <div className="muted">-</div>}
          <h3 className="mt">{t("Related cases")}</h3>
          {data.related?.length ? (
            <table className="tbl"><tbody>{data.related.map((k) => (
              <tr key={k.id}><td><Link href={`/cases/${k.id}`}>{k.number}</Link></td><td>{k.title}</td><td><Status s={k.status} /></td>
                <td className="r"><button className="link danger" onClick={() => act(api.post(`/api/cases/${c.id}/related`, { other: k.id, link: false }))}>{t("Unlink")}</button></td></tr>
            ))}</tbody></table>
          ) : <div className="muted">-</div>}
          <div className="row3 mt">
            <input placeholder={t("Case no.")} value={relate} onChange={(x) => setRelate(x.target.value)} />
            <div><button className="btn-s" disabled={!relate} onClick={() => act(api.post(`/api/cases/${c.id}/related`, { other: relate.trim(), link: true })).then(() => setRelate(""))}>{t("Link case")}</button></div>
          </div>
        </section>
      )}

      {tab === "timeline" && (
        <section className="card">
          <ol className="timeline">
            {timeline.map((x, i) => (
              <li key={i}><span className="small muted">{dt(x.at)}<br />{x.by}</span><span className={x.stage ? "tl-stage" : ""}>{x.text}</span></li>
            ))}
          </ol>
        </section>
      )}

      {tab === "flow" && (
        <section className="card">
          <Viewer xml={data.version.bpmn} active={(c.instance.tokens ?? []).map((x) => x.node)}
            done={Array.from(new Set(c.instance.history.filter((h) => h.event === "leave").map((h) => h.node)))} />
        </section>
      )}

      {action && (
        <Modal title={action.label} onClose={() => setAction(undefined)} footer={<>
          <button className="btn-o" style={{ border: 0, background: "var(--cds-button-secondary)", color: "#fff" }} onClick={() => setAction(undefined)}>{t("Cancel")}</button>
          <button className={action.kind === "stage" ? "btn danger-fill" : "btn"} onClick={runAction}>{action.label}</button>
        </>}>
          {action.kind === "stage" && <p>{t("The case moves to stage")} <b>{stageName(action.target)}</b>. {t("Open work in the current stage is cancelled.")}</p>}
          {action.kind === "child" && <p>{t("A new case is opened and linked to this one.")}</p>}
          {aform && <FormRenderer fields={aform.schema.fields} pages={aform.schema.pages} data={adata} onChange={setAdata} errors={aerr} />}
          {aerr._ && <div className="err">{aerr._}</div>}
        </Modal>
      )}
    </Page>
  );
}

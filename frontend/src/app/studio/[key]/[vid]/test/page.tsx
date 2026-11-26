"use client";
// Test run: start a sandbox case of this version (drafts too), act as any
// role, step through the flow and watch stage, data and tokens change.
// Test cases never appear in real work lists or reports.
import { useParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { api, Case, Form, StageView, Version, Work } from "@/lib/api";
import { dt } from "@/lib/format";
import { t } from "@/lib/i18n";
import { useLoad } from "@/lib/useLoad";
import FormRenderer, { validate } from "@/components/FormRenderer";
import StageBar from "@/components/StageBar";
import { Viewer } from "@/components/Bpmn";
import { Err, Field, Status } from "@/components/ui";

type View = { case: Case; work: Work[] | null; stages?: StageView[] | null; events?: { at: string; type: string; node?: string }[] | null; incidents?: { message: string; status: string; nodeName: string }[] | null };

export default function TestRun() {
  const { key, vid } = useParams<{ key: string; vid: string }>();
  const { data: v } = useLoad(() => api.get<Version>(`/api/versions/${vid}`), [vid]);
  const { data: runs, reload: reloadRuns } = useLoad(() => api.get<Case[]>(`/api/cases?process=${key}&version=${vid}&test=1`), [key, vid]);
  const [caseId, setCaseId] = useState<string>();
  const [view, setView] = useState<View>();
  const [forms, setForms] = useState<Record<string, Form>>({});
  const [data, setData] = useState<Record<string, Record<string, any>>>({});
  const [errs, setErrs] = useState<Record<string, Record<string, string>>>({});
  const [vars, setVars] = useState("{}");
  const [msgName, setMsgName] = useState("");
  const [e, setE] = useState<string>();

  const load = useCallback(async (id = caseId) => {
    if (!id) return;
    const d = await api.get<View>(`/api/cases/${id}`);
    setView(d);
    for (const w of d.work ?? []) {
      if (w.formKey && !forms[w.formKey]) {
        const f = await api.get<Form>(`/api/forms/${w.formKey}`).catch(() => undefined);
        if (f) setForms((x) => ({ ...x, [w.formKey!]: f }));
      }
    }
  }, [caseId, forms]);
  useEffect(() => { load(); }, [caseId]); // eslint-disable-line react-hooks/exhaustive-deps

  // jobs (decisions, integrations) run in the background worker: refresh shortly after each action
  const after = () => { load(); setTimeout(() => { load(); reloadRuns(); }, 1500); };
  const start = async () => {
    try {
      const c = await api.post<Case>("/api/cases", { processKey: key, versionId: vid, test: true, title: `Test ${new Date().toLocaleTimeString()}`, vars: JSON.parse(vars || "{}") });
      setCaseId(c.id); setE(undefined); reloadRuns(); setTimeout(() => load(c.id), 1500);
    } catch (x: any) { setE(x.message); }
  };
  const complete = async (w: Work) => {
    const f = w.formKey ? forms[w.formKey] : undefined;
    const d = data[w.id] ?? {};
    const v = validate(f?.schema.fields ?? [], d);
    setErrs({ ...errs, [w.id]: v });
    if (Object.keys(v).length) return;
    try { await api.post(`/api/work/${w.id}/complete`, d); setE(undefined); after(); } catch (x: any) { setE(x.message); }
  };
  const send = async () => {
    try { const r = await api.post<{ matched: number }>("/api/messages", { name: msgName, caseId }); setE(r.matched ? undefined : t("No step is waiting for this message")); after(); } catch (x: any) { setE(x.message); }
  };

  const c = view?.case;
  const open = (view?.work ?? []).filter((w) => w.status === "open" || w.status === "claimed");
  const waiting = (c?.instance.tokens ?? []).filter((x) => x.wait === "message");

  return (
    <div>
      <div className="page-head"><h1>{t("Test run")}</h1></div>
      <Err e={e} />
      <div className="studio-split">
        <div>
          {!c ? <div className="empty muted">—</div> : (
            <>
              <div className="case-head">
                <div className="kv1"><span>{t("Case")}</span><span>{c.number}</span></div>
                <div className="kv1"><span>{t("Status")}</span><Status s={c.status} /></div>
                <div className="kv1"><span>{t("Stage")}</span><span>{c.stage ?? "-"}</span></div>
              </div>
              <StageBar stages={view?.stages ?? []} />
              {(view?.incidents ?? []).filter((i) => i.status === "open").map((i, k) => <div key={k} className="err">{i.nodeName}: {i.message}</div>)}
              {open.map((w) => (
                <section key={w.id} className="card mt">
                  <h3>{w.name} <span className="small muted">· {w.role || "-"}</span></h3>
                  {w.formKey && forms[w.formKey]
                    ? <FormRenderer fields={forms[w.formKey].schema.fields} data={data[w.id] ?? {}} onChange={(d) => setData({ ...data, [w.id]: d })} errors={errs[w.id]} />
                    : <Field label={t("Data (JSON)")}><textarea className="mono" rows={3} placeholder='{"approve_approved": true}' onChange={(x) => { try { setData({ ...data, [w.id]: JSON.parse(x.target.value || "{}") }); } catch {} }} /></Field>}
                  <div className="bar"><button className="btn" onClick={() => complete(w)}>{t("Complete step")}</button></div>
                </section>
              ))}
              {waiting.length > 0 && (
                <section className="card mt">
                  <h3>{t("Waiting for a message")}</h3>
                  <div className="row-url">
                    <button className="btn-o" onClick={send}>{t("Send")}</button>
                    <input value={msgName} placeholder={waiting.map((x) => x.node).join(", ")} onChange={(x) => setMsgName(x.target.value)} />
                  </div>
                </section>
              )}
              {v && <section className="card mt"><Viewer xml={v.bpmn} active={(c.instance.tokens ?? []).map((x) => x.node)} /></section>}
            </>
          )}
        </div>
        <aside className="panel">
          <h3>{t("New test case")}</h3>
          <Field label={t("Start data (JSON)")}><textarea className="mono" rows={4} value={vars} onChange={(x) => setVars(x.target.value)} /></Field>
          <button className="btn" onClick={start}>{t("Start test")}</button>
          {c && <>
            <div className="fld-l mt">{t("Case data")}</div>
            <pre className="code">{JSON.stringify(c.instance.vars, null, 2)}</pre>
            <div className="fld-l mt">{t("Tokens")}</div>
            <pre className="code">{JSON.stringify(c.instance.tokens, null, 2)}</pre>
          </>}
          {(runs ?? []).length > 0 && <>
            <div className="fld-l mt">{t("Previous runs")}</div>
            {(runs ?? []).slice(0, 8).map((r) => (
              <button key={r.id} className={"pal" + (r.id === caseId ? " on" : "")} onClick={() => setCaseId(r.id)}>{r.number} · {r.status} · {dt(r.createdAt)}</button>
            ))}
          </>}
        </aside>
      </div>
    </div>
  );
}

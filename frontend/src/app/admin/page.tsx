"use client";
// Import hub: BPMN (any modeller), solution pack (.cmpack/.zip), form JSON,
// cases / master data (CSV, XLSX). Always previews before committing.
import { useState } from "react";
import { api, ProcessDef } from "@/lib/api";
import { dt } from "@/lib/format";
import { useLoad } from "@/lib/useLoad";
import Dropdown from "@/components/Dropdown";
import DropZone from "@/components/DropZone";
import { Err, Field, Page } from "@/components/ui";

const KINDS = [
  { value: "", label: "Detect from file" }, { value: "bpmn", label: "BPMN 2.0 (Camunda, Bizagi, draw.io, ...)" },
  { value: "pack", label: "Solution pack (.cmpack)" }, { value: "form", label: "Form (JSON)" },
  { value: "cases", label: "Cases / master data (CSV, Excel)" },
];
type Preview = { preview: boolean; summary: Record<string, any>; warnings?: string[]; bundle?: any };

export default function Import() {
  const { data: procs } = useLoad(() => api.get<ProcessDef[]>("/api/processes"));
  const { data: jobs, reload } = useLoad(() => api.get<any[]>("/api/imports"));
  const [file, setFile] = useState<File>();
  const [kind, setKind] = useState("");
  const [key, setKey] = useState("");
  const [fy, setFy] = useState("");
  const [p, setP] = useState<Preview>();
  const [e, setE] = useState<string>();
  const [msg, setMsg] = useState<string>();
  const send = async (commit: boolean) => {
    if (!file) return;
    const fd = new FormData();
    fd.append("file", file);
    fd.append("kind", kind);
    fd.append("processKey", key);
    if (fy) fd.append("fiscalYear", String(Number(fy)));
    if (commit) fd.append("commit", "1");
    setE(undefined); setMsg(undefined);
    try {
      const r = await api.upload<any>("/api/imports", fd);
      if (commit) { setMsg("Import completed"); setP(undefined); reload(); } else setP(r);
    } catch (x: any) { setE(x.message); }
  };
  const isCases = kind === "cases" || /\.(csv|xlsx)$/i.test(file?.name ?? "");
  return (
    <Page title="Import">
      <div className="card">
        <div className="row3">
          <Field label="Source type"><Dropdown options={KINDS} value={kind} onChange={setKind} /></Field>
          <Field label={isCases ? "Target case type" : "Case type key (optional)"}>
            <Dropdown allowCustom options={(procs ?? []).map((x) => ({ value: x.key, label: x.name }))} value={key} onChange={setKey} />
          </Field>
          <Field label="Fiscal year"><input inputMode="numeric" value={fy} onChange={(x) => setFy(x.target.value)} /></Field>
        </div>
        <DropZone accept=".bpmn,.xml,.zip,.cmpack,.json,.csv,.xlsx" onFiles={(f) => { setFile(f[0]); setP(undefined); }} />
        <div className="bar"><button className="btn-o" disabled={!file} onClick={() => send(false)}>Preview import</button></div>
        <Err e={e} />{msg && <div className="ok">{msg}</div>}
        {p && (
          <div className="mt">
            <h3>Preview result</h3>
            <table className="tbl"><tbody>
              <tr><td>Case types / flows</td><td>{(p.summary.processes ?? []).join(", ") || "-"}</td></tr>
              <tr><td>Forms</td><td>{p.summary.forms}</td></tr>
              <tr><td>Screens</td><td>{p.summary.screens}</td></tr>
              <tr><td>Cases to create</td><td>{p.summary.cases}</td></tr>
            </tbody></table>
            {(p.warnings ?? []).map((w) => <div key={w} className="warn">{w}</div>)}
            {p.bundle?.cases?.length > 0 && (
              <table className="tbl mt">
                <thead><tr><th>#</th><th>Title</th><th>Fiscal year</th><th>Data</th></tr></thead>
                <tbody>{p.bundle.cases.slice(0, 20).map((c: any, i: number) => (
                  <tr key={i}><td>{i + 1}</td><td>{c.title}</td><td>{c.fiscalYear ? c.fiscalYear : "-"}</td><td className="small">{JSON.stringify(c.vars)}</td></tr>
                ))}</tbody>
              </table>
            )}
            <div className="bar"><button className="btn" onClick={() => send(true)}>Import</button></div>
          </div>
        )}
      </div>
      <section className="card mt">
        <h3>Import history</h3>
        <table className="tbl">
          <thead><tr><th>Time</th><th>File</th><th>Result</th><th>Summary</th><th>Imported by</th></tr></thead>
          <tbody>{(jobs ?? []).map((j) => (
            <tr key={j.id}><td className="nw">{dt(j.createdAt)}</td><td>{j.fileName}</td><td>{j.status === "committed" ? "Succeeded" : "Failed"}</td>
              <td className="small">{j.issues?.join("; ") || `flow ${j.summary?.processes?.length ?? 0} · Forms ${j.summary?.forms ?? 0} · Case ${j.summary?.casesCreated ?? 0}`}</td><td>{j.createdBy}</td></tr>
          ))}</tbody>
        </table>
      </section>
    </Page>
  );
}

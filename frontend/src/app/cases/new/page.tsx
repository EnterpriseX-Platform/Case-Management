"use client";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { api, Case, ProcessDef, Version } from "@/lib/api";
import { useLoad } from "@/lib/useLoad";
import Dropdown from "@/components/Dropdown";
import { Err, Field, Page } from "@/components/ui";

export default function NewCase() {
  const r = useRouter();
  const { data: procs } = useLoad(() => api.get<ProcessDef[]>("/api/processes"));
  const [key, setKey] = useState("");
  const { data: vers } = useLoad(() => (key ? api.get<Version[]>(`/api/processes/${key}/versions`) : Promise.resolve([])), [key]);
  const years = (vers ?? []).filter((v) => v.status === "published").map((v) => v.fiscalYear!).sort();
  const [fy, setFy] = useState("");
  const [title, setTitle] = useState("");
  const [e, setE] = useState<string>();
  const start = () =>
    api.post<Case>("/api/cases", { processKey: key, fiscalYear: Number(fy), title })
      .then((c) => r.push(`/cases/${c.id}`)).catch((x) => setE(x.message));
  return (
    <Page title="New case">
      <div className="card narrow">
        <Field label="Case type">
          <Dropdown value={key} onChange={(v) => { setKey(v); setFy(""); }} options={(procs ?? []).map((p) => ({ value: p.key, label: p.name }))} />
        </Field>
        <Field label="Fiscal year">
          <Dropdown value={fy} onChange={setFy} disabled={!key} options={years.map((y) => ({ value: String(y), label: String(y) }))} />
        </Field>
        <Field label="Title"><input value={title} onChange={(e) => setTitle(e.target.value)} /></Field>
        <Err e={e} />
        <div className="bar"><button className="btn" disabled={!key || !fy} onClick={start}>Create case</button></div>
      </div>
    </Page>
  );
}

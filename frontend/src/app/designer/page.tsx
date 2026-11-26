"use client";
import Link from "next/link";
import { useState } from "react";
import { api, ProcessDef } from "@/lib/api";
import { useLoad } from "@/lib/useLoad";
import Dropdown from "@/components/Dropdown";
import { Empty, Err, Field, Page } from "@/components/ui";

export default function Designer() {
  const { data, error, reload } = useLoad(() => api.get<ProcessDef[]>("/api/processes"));
  const [n, setN] = useState<Partial<ProcessDef> | null>(null);
  const [e, setE] = useState<string>();
  return (
    <Page title="Case types & flows" actions={<button className="btn" onClick={() => setN({})}>New case type</button>}>
      <Err e={error ?? e} />
      {n && (
        <div className="card">
          <div className="row3">
            <Field label="Key (lowercase)"><input value={n.key ?? ""} onChange={(x) => setN({ ...n, key: x.target.value.toLowerCase() })} /></Field>
            <Field label="Name"><input value={n.name ?? ""} onChange={(x) => setN({ ...n, name: x.target.value })} /></Field>
            <Field label="Inherits from"><Dropdown value={n.parentKey} onChange={(v) => setN({ ...n, parentKey: v })} options={(data ?? []).map((p) => ({ value: p.key, label: p.name }))} /></Field>
          </div>
          <div className="bar">
            <button className="btn" onClick={() => api.post("/api/processes", n).then(() => { setN(null); reload(); }).catch((x) => setE(x.message))}>Save</button>
            <button className="link" onClick={() => setN(null)}>Cancel</button>
          </div>
        </div>
      )}
      {data?.length ? (
        <table className="tbl">
          <thead><tr><th>Key</th><th>Name</th><th>Category</th><th>Inherits from</th></tr></thead>
          <tbody>
            {data.map((p) => (
              <tr key={p.key}><td><Link href={`/designer/${p.key}`}>{p.key}</Link></td><td>{p.name}</td><td>{p.category ?? "-"}</td><td>{p.parentKey ?? "-"}</td></tr>
            ))}
          </tbody>
        </table>
      ) : data && <Empty>No case types yet</Empty>}
    </Page>
  );
}

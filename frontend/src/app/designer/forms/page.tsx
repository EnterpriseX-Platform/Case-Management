"use client";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { api, Form } from "@/lib/api";
import { dt } from "@/lib/format";
import { useLoad } from "@/lib/useLoad";
import { Err, Page } from "@/components/ui";

export default function Forms() {
  const r = useRouter();
  const { data, error } = useLoad(() => api.get<Form[]>("/api/forms"));
  return (
    <Page title="Forms" actions={<button className="btn" onClick={() => { const k = prompt("Form key"); if (k) r.push(`/designer/forms/${k}`); }}>New form</button>}>
      <Err e={error} />
      <table className="tbl">
        <thead><tr><th>Key</th><th>Name</th><th>Fields</th><th>Version</th><th>Updated</th></tr></thead>
        <tbody>{(data ?? []).map((f: any) => (
          <tr key={f.key}><td><Link href={`/designer/forms/${f.key}`}>{f.key}</Link></td><td>{f.name}</td><td>{f.schema?.fields?.length ?? 0}</td><td>{f.version}</td><td className="nw">{dt(f.updatedAt)}</td></tr>
        ))}</tbody>
      </table>
    </Page>
  );
}

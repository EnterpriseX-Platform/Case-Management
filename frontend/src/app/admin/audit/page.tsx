"use client";
import { useState } from "react";
import { api, Audit } from "@/lib/api";
import { dt } from "@/lib/format";
import { useLoad } from "@/lib/useLoad";
import { auditText } from "@/components/ScreenRenderer";
import { Err, Page } from "@/components/ui";

export default function AuditPage() {
  const [ref, setRef] = useState("");
  const { data, error } = useLoad(() => api.get<Audit[]>(`/api/audit?ref=${encodeURIComponent(ref)}`), [ref]);
  return (
    <Page title="Audit log">
      <div className="filters"><input placeholder="Reference (case / version)" value={ref} onChange={(e) => setRef(e.target.value)} /></div>
      <Err e={error} />
      <table className="tbl">
        <thead><tr><th>Time</th><th>User</th><th>Action</th><th>Event</th><th>Reference</th></tr></thead>
        <tbody>{(data ?? []).map((a, i) => (
          <tr key={i}><td className="nw">{dt(a.at)}</td><td>{a.actor}</td><td className="small">{a.action}</td><td>{auditText(a)}</td><td className="small muted">{a.entity}:{a.ref}</td></tr>
        ))}</tbody>
      </table>
    </Page>
  );
}

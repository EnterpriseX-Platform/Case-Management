"use client";
// App Studio home: one tile row per case type (app); create a new app.
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { api, ProcessDef, Version } from "@/lib/api";
import { t } from "@/lib/i18n";
import { useLoad } from "@/lib/useLoad";
import { Err, Field, Page, Status } from "@/components/ui";

const BLANK = `<?xml version="1.0" encoding="UTF-8"?><definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="start"/><endEvent id="end"/><sequenceFlow id="f1" sourceRef="start" targetRef="end"/></process></definitions>`;
const STARTER = {
  stages: [
    { key: "submit", name: "Submit", kind: "primary", steps: [{ key: "submit-request", name: "Submit request", kind: "task", role: "requester" }] },
    { key: "review", name: "Review", kind: "primary", sla: "P5D", steps: [{ key: "approve", name: "Approve request", kind: "approval", role: "approver", onReject: "end" }] },
    { key: "resolved", name: "Resolved", kind: "resolution", steps: [] },
  ],
};

export default function Studio() {
  const r = useRouter();
  const { data, error } = useLoad(async () => {
    const ps = await api.get<ProcessDef[]>("/api/processes");
    return Promise.all((ps ?? []).map(async (p) => ({ p, v: ((await api.get<Version[]>(`/api/processes/${p.key}/versions`)) ?? [])[0] })));
  });
  const [n, setN] = useState<{ key: string; name: string } | null>(null);
  const [e, setE] = useState<string>();
  const create = async () => {
    if (!n) return;
    try {
      await api.post("/api/processes", { key: n.key, name: n.name });
      const res = await api.post<{ version: Version }>(`/api/processes/${n.key}/versions`, { bpmn: BLANK, fiscalYear: new Date().getFullYear() + 1 });
      await api.put(`/api/versions/${res.version.id}/lifecycle`, STARTER);
      r.push(`/studio/${n.key}/${res.version.id}/lifecycle`);
    } catch (x: any) { setE(x.message); }
  };
  return (
    <Page title={t("App Studio")} actions={<button className="btn" onClick={() => setN({ key: "", name: "" })}>{t("New app")}</button>}>
      <Err e={error ?? e} />
      {n && (
        <div className="card">
          <div className="row3">
            <Field label={t("Name")}><input autoFocus value={n.name} onChange={(x) => setN({ name: x.target.value, key: n.key || x.target.value.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") })} /></Field>
            <Field label={t("Key (lowercase)")}><input className="mono" value={n.key} onChange={(x) => setN({ ...n, key: x.target.value.toLowerCase() })} /></Field>
          </div>
          <div className="bar"><button className="btn" disabled={!n.key || !n.name} onClick={create}>{t("Create")}</button><button className="link" onClick={() => setN(null)}>{t("Cancel")}</button></div>
        </div>
      )}
      <table className="tbl">
        <thead><tr><th>{t("App")}</th><th>{t("Key")}</th><th>{t("Latest version")}</th><th>{t("Status")}</th><th>{t("Fiscal year")}</th></tr></thead>
        <tbody>{(data ?? []).map(({ p, v }) => (
          <tr key={p.key}>
            <td>{v ? <Link href={`/studio/${p.key}/${v.id}/lifecycle`}>{p.name}</Link> : p.name}</td>
            <td className="mono small">{p.key}</td><td>{v ? `v${v.no}` : "-"}</td><td>{v && <Status s={v.status} />}</td><td>{v?.fiscalYear ?? "-"}</td>
          </tr>
        ))}</tbody>
      </table>
    </Page>
  );
}

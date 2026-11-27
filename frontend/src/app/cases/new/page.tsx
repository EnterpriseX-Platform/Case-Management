"use client";
import { t } from "@/lib/i18n";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { api, Case, ProcessDef, Version } from "@/lib/api";
import { useLoad } from "@/lib/useLoad";
import Dropdown from "@/components/Dropdown";
import { Err, Field, Page } from "@/components/ui";

export default function NewCase() {
  const r = useRouter();
  const { data: procs } = useLoad(() => api.get<ProcessDef[]>("/api/processes"));
  const [key, setKey] = useState(() => (typeof window !== "undefined" ? new URLSearchParams(location.search).get("type") ?? "" : ""));
  const { data: vers } = useLoad(() => (key ? api.get<Version[]>(`/api/processes/${key}/versions`) : Promise.resolve([])), [key]);
  // fiscal years only matter for case types that publish one version per year
  const years = Array.from(new Set((vers ?? []).filter((v) => v.status === "published" && v.fiscalYear).map((v) => v.fiscalYear!))).sort();
  const byYear = years.length > 0;
  const [fy, setFy] = useState("");
  const [title, setTitle] = useState("");
  const [e, setE] = useState<string>();
  const start = () =>
    api.post<Case>("/api/cases", { processKey: key, fiscalYear: byYear ? Number(fy) : 0, title })
      .then((c) => r.push(`/cases/${c.id}`)).catch((x) => setE(x.message));
  return (
    <Page title={t("New case")}>
      <div className="card narrow">
        <Field label={t("Case type")}>
          <Dropdown value={key} onChange={(v) => { setKey(v); setFy(""); }} options={(procs ?? []).map((p) => ({ value: p.key, label: p.name }))} />
        </Field>
        {byYear && (
          <Field label={t("Fiscal year")}>
            <Dropdown value={fy} onChange={setFy} options={years.map((y) => ({ value: String(y), label: String(y) }))} />
          </Field>
        )}
        <Field label={t("Title")}><input value={title} onChange={(e) => setTitle(e.target.value)} /></Field>
        <Err e={e} />
        <div className="bar"><button className="btn" disabled={!key || (byYear && !fy)} onClick={start}>{t("Create case")}</button></div>
      </div>
    </Page>
  );
}

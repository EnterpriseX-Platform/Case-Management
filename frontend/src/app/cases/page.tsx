"use client";
import { t } from "@/lib/i18n";
import Link from "next/link";
import { useState } from "react";
import { api, Case, ProcessDef } from "@/lib/api";
import { dt } from "@/lib/format";
import { useLoad } from "@/lib/useLoad";
import Dropdown from "@/components/Dropdown";
import { Empty, Err, Page, Status } from "@/components/ui";

export default function Cases() {
  const [proc, setProc] = useState("");
  const [fy, setFy] = useState("");
  const [st, setSt] = useState("");
  const { data: procs } = useLoad(() => api.get<ProcessDef[]>("/api/processes"));
  const q = new URLSearchParams({ process: proc, fiscalYear: fy, status: st }).toString();
  const { data, error } = useLoad(() => api.get<Case[]>(`/api/cases?${q}`), [q]);
  const years = Array.from(new Set((data ?? []).map((c) => c.fiscalYear).filter(Boolean)));
  return (
    <Page title={t("All cases")} actions={<Link className="btn" href="/cases/new">{t("New case")}</Link>}>
      <div className="filters">
        <Dropdown placeholder={t("All case types")} value={proc} onChange={setProc} options={(procs ?? []).map((p) => ({ value: p.key, label: p.name }))} />
        <Dropdown placeholder={t("All fiscal years")} value={fy} onChange={setFy} allowCustom options={years.map((y) => ({ value: String(y), label: String(y!) }))} />
        <Dropdown placeholder={t("All statuses")} value={st} onChange={setSt} options={["running", "completed", "cancelled", "failed"].map((x) => ({ value: x, label: x === "running" ? "In progress" : x === "completed" ? "Completed" : x === "cancelled" ? "Cancel" : "Failed" }))} />
      </div>
      <Err e={error} />
      {data?.length ? (
        <table className="tbl">
          <thead><tr><th>{t("No.")}</th><th>{t("Case")}</th><th>{t("Type")}</th><th>{t("Fiscal year")}</th><th>{t("Current step")}</th><th>{t("Status")}</th><th>{t("Last updated")}</th></tr></thead>
          <tbody>
            {data.map((c) => (
              <tr key={c.id}>
                <td className="nw"><Link href={`/cases/${c.id}`}>{c.number}</Link></td>
                <td>{c.title}</td>
                <td>{procs?.find((p) => p.key === c.processKey)?.name ?? c.processKey}</td>
                <td>{c.fiscalYear ? c.fiscalYear : "-"}</td>
                <td>{Array.from(new Set((c.instance.tokens ?? []).map((t) => t.node))).join(", ") || "-"}</td>
                <td><Status s={c.status} /></td>
                <td className="nw">{dt(c.updatedAt)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : data && <Empty>{t("No cases")}</Empty>}
    </Page>
  );
}

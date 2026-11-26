"use client";
// Incident queue: steps parked by a technical failure (connector down,
// script error, ...). Operators retry, fix data, move the case or cancel it.
import { t } from "@/lib/i18n";
import { useDialog } from "@/components/useDialog";
import Link from "next/link";
import { Fragment, useState } from "react";
import { api } from "@/lib/api";
import { dt } from "@/lib/format";
import { useLoad } from "@/lib/useLoad";
import Dropdown from "@/components/Dropdown";
import { Empty, Err, Page, Status } from "@/components/ui";

type Incident = {
  id: string; caseId: string; caseNumber: string; processKey: string; tokenId: string; node: string; nodeName: string;
  message: string; status: string; createdAt: string; resolvedAt?: string; resolvedBy?: string; resolution?: string;
};

export default function Incidents() {
  const [st, setSt] = useState("open");
  const { data, error, reload } = useLoad(() => api.get<Incident[]>(`/api/incidents?status=${st}`), [st]);
  const [e, setE] = useState<string>();
  const [msg, setMsg] = useState<string>();
  const [open, setOpen] = useState<string>();
  const dlg = useDialog();
  const [vars, setVars] = useState("");
  const act = async (i: Incident, action: string) => {
    setE(undefined); setMsg(undefined);
    try {
      let v: Record<string, unknown> | undefined;
      if (vars.trim()) v = JSON.parse(vars);
      await api.post(`/api/incidents/${i.id}/${action}`, { vars: v });
      setMsg(`${i.caseNumber}: ${action === "retry" ? "retried" : "cancelled"}`);
      setOpen(undefined); setVars("");
      reload();
    } catch (x: any) { setE(x.message); }
  };
  return (
    <Page title={t("Incidents")} actions={st === "open" && (data?.length ?? 0) > 0 &&
      <button className="btn-o" onClick={() => api.post<{ retried: number }>("/api/incidents-retry-all").then((r) => { setMsg(`Retried ${r.retried}`); reload(); }).catch((x) => setE(x.message))}>{t("Retry all")}</button>}>
      {dlg.node}
      <div className="filters">
        <Dropdown value={st} onChange={setSt} options={[{ value: "open", label: t("Open") }, { value: "resolved", label: t("Resolved") }, { value: "", label: t("All") }]} />
      </div>
      <Err e={error ?? e} />{msg && <div className="ok">{msg}</div>}
      {data?.length ? (
        <table className="tbl">
          <thead><tr><th>{t("Raised")}</th><th>{t("Case")}</th><th>{t("Step")}</th><th>{t("Error")}</th><th>{t("Status")}</th><th /></tr></thead>
          <tbody>
            {data.map((i) => (
              <Fragment key={i.id}>
                <tr>
                  <td className="nw">{dt(i.createdAt)}</td>
                  <td className="nw"><Link href={`/cases/${i.caseId}`}>{i.caseNumber}</Link></td>
                  <td>{i.nodeName}</td>
                  <td className="mono small">{i.message}</td>
                  <td>{i.status === "open" ? <Status s="failed" /> : <span className="small">{i.resolution} · {i.resolvedBy ?? "system"}</span>}</td>
                  <td className="nw">
                    {i.status === "open" && <>
                      <button className="btn-s" onClick={() => act(i, "retry")}>{t("Retry")}</button>
                      <button className="link" onClick={() => setOpen(open === i.id ? undefined : i.id)}>{t("Fix data")}</button>
                      <button className="link danger" onClick={async () => (await dlg.confirm(t("Cancel the whole case?"), { ok: t("Cancel case"), danger: true })) && act(i, "cancel")}>{t("Cancel case")}</button>
                    </>}
                  </td>
                </tr>
                {open === i.id && (
                  <tr><td colSpan={6}>
                    <div className="row2">
                      <textarea rows={3} className="mono" placeholder='{"amount": 1000}' value={vars} onChange={(x) => setVars(x.target.value)} />
                      <div><button className="btn" onClick={() => act(i, "retry")}>{t("Save and retry")}</button></div>
                    </div>
                  </td></tr>
                )}
              </Fragment>
            ))}
          </tbody>
        </table>
      ) : data && <Empty>{t("No incidents")}</Empty>}
    </Page>
  );
}

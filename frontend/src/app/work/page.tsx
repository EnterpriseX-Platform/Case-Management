"use client";
import { t } from "@/lib/i18n";
import Link from "next/link";
import { useState } from "react";
import { api, Work } from "@/lib/api";
import { dt, loopLabel, overdue } from "@/lib/format";
import { useLoad } from "@/lib/useLoad";
import { useSession } from "@/lib/useSession";
import { Empty, Err, Page, Status } from "@/components/ui";

export default function Inbox() {
  const s = useSession();
  const { data, error, reload } = useLoad(() => api.get<Work[]>("/api/work"), [s?.id]);
  const [tab, setTab] = useState<"mine" | "queue">("mine");
  const [e, setE] = useState<string>();
  const list = (data ?? []).filter((w) => (tab === "mine" ? w.assignee === s?.id : !w.assignee));
  return (
    <Page title={t("Inbox")}>
      <div className="tabs">
        <button className={tab === "mine" ? "on" : ""} onClick={() => setTab("mine")}>My work ({(data ?? []).filter((w) => w.assignee === s?.id).length})</button>
        <button className={tab === "queue" ? "on" : ""} onClick={() => setTab("queue")}>Role queue ({(data ?? []).filter((w) => !w.assignee).length})</button>
      </div>
      <Err e={error ?? e} />
      {list.length ? (
        <table className="tbl">
          <thead><tr><th>{t("Case no.")}</th><th>{t("Case")}</th><th>{t("Step")}</th><th>{t("Role")}</th><th>{t("Received")}</th><th>{t("Due")}</th><th>{t("Status")}</th><th /></tr></thead>
          <tbody>
            {list.map((w) => (
              <tr key={w.id} className={overdue(w.dueAt) ? "late" : ""}>
                <td className="nw">{w.caseNumber}</td>
                <td>{w.caseTitle}</td>
                <td><Link href={`/work/${w.id}`}>{w.name}</Link><span className="muted">{loopLabel(w.input)}</span></td>
                <td>{w.role}</td>
                <td className="nw">{dt(w.createdAt)}</td>
                <td className="nw">{dt(w.dueAt)}{overdue(w.dueAt) && <span className="late-t"> {t("Overdue")}</span>}</td>
                <td><Status s={w.status} /></td>
                <td className="nw">
                  {tab === "queue"
                    ? <button className="btn-s" onClick={() => api.post(`/api/work/${w.id}/claim`).then(reload).catch((x) => setE(x.message))}>{t("Claim")}</button>
                    : <Link className="btn-s" href={`/work/${w.id}`}>{t("Open")}</Link>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : data && <Empty>{t("No work items")}</Empty>}
    </Page>
  );
}

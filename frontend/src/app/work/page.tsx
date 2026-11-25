"use client";
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
    <Page title="กล่องงาน">
      <div className="tabs">
        <button className={tab === "mine" ? "on" : ""} onClick={() => setTab("mine")}>งานของฉัน ({(data ?? []).filter((w) => w.assignee === s?.id).length})</button>
        <button className={tab === "queue" ? "on" : ""} onClick={() => setTab("queue")}>คิวงานตามบทบาท ({(data ?? []).filter((w) => !w.assignee).length})</button>
      </div>
      <Err e={error ?? e} />
      {list.length ? (
        <table className="tbl">
          <thead><tr><th>เลขที่เรื่อง</th><th>เรื่อง</th><th>ขั้นตอน</th><th>บทบาท</th><th>รับเข้า</th><th>กำหนดเสร็จ</th><th>สถานะ</th><th /></tr></thead>
          <tbody>
            {list.map((w) => (
              <tr key={w.id} className={overdue(w.dueAt) ? "late" : ""}>
                <td className="nw">{w.caseNumber}</td>
                <td>{w.caseTitle}</td>
                <td><Link href={`/work/${w.id}`}>{w.name}</Link><span className="muted">{loopLabel(w.input)}</span></td>
                <td>{w.role}</td>
                <td className="nw">{dt(w.createdAt)}</td>
                <td className="nw">{dt(w.dueAt)}{overdue(w.dueAt) && <span className="late-t"> เกินกำหนด</span>}</td>
                <td><Status s={w.status} /></td>
                <td className="nw">
                  {tab === "queue"
                    ? <button className="btn-s" onClick={() => api.post(`/api/work/${w.id}/claim`).then(reload).catch((x) => setE(x.message))}>รับงาน</button>
                    : <Link className="btn-s" href={`/work/${w.id}`}>ทำงาน</Link>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : data && <Empty>ไม่มีงาน</Empty>}
    </Page>
  );
}

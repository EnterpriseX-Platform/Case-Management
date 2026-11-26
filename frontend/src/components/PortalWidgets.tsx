"use client";
// Widgets for persona home pages (portal screens).
import Link from "next/link";
import { api, Case, ProcessDef, Work } from "@/lib/api";
import { dt, loopLabel, overdue } from "@/lib/format";
import { t } from "@/lib/i18n";
import { useLoad } from "@/lib/useLoad";
import { Status } from "./ui";

export function MyWork() {
  const { data } = useLoad(() => api.get<Work[]>("/api/work"));
  const list = (data ?? []).slice(0, 8);
  return (
    <div>
      <h3>{t("My work")} <span className="muted small">({(data ?? []).length})</span></h3>
      {list.length ? (
        <table className="tbl"><tbody>{list.map((w) => (
          <tr key={w.id} className={overdue(w.dueAt) ? "late" : ""}>
            <td><Link href={`/work/${w.id}`}>{w.name}</Link>{loopLabel(w.input)}<div className="small muted">{w.caseNumber}</div></td>
            <td className="nw small">{dt(w.dueAt)}</td>
          </tr>
        ))}</tbody></table>
      ) : <div className="muted">-</div>}
      <div className="bar"><Link href="/work" className="link">{t("Inbox")} →</Link></div>
    </div>
  );
}

export function MyCases() {
  const { data } = useLoad(() => api.get<Case[]>("/api/cases?mine=1&limit=50"));
  return (
    <div>
      <h3>{t("My cases")}</h3>
      {(data ?? []).length ? (
        <table className="tbl"><tbody>{(data ?? []).slice(0, 8).map((c) => (
          <tr key={c.id}><td><Link href={`/cases/${c.id}`}>{c.number}</Link><div className="small muted">{c.title}</div></td><td>{c.stage ?? ""}</td><td><Status s={c.status} /></td></tr>
        ))}</tbody></table>
      ) : <div className="muted">-</div>}
    </div>
  );
}

export function NewCase() {
  const { data } = useLoad(() => api.get<ProcessDef[]>("/api/processes"));
  return (
    <div>
      <h3>{t("New case")}</h3>
      <div className="tiles">{(data ?? []).map((p) => (
        <Link key={p.key} className="tile" href={`/cases/new?type=${p.key}`}>{p.name}</Link>
      ))}</div>
    </div>
  );
}

export function KPIs() {
  const { data } = useLoad(() => api.get<Work[]>("/api/work"));
  const w = data ?? [];
  const late = w.filter((x) => overdue(x.dueAt)).length;
  const today = w.filter((x) => x.dueAt && new Date(x.dueAt).toDateString() === new Date().toDateString()).length;
  return (
    <table className="tbl"><tbody>
      <tr><td>{t("Open work")}</td><td className="r">{w.length}</td></tr>
      <tr><td>{t("Due today")}</td><td className="r">{today}</td></tr>
      <tr><td>{t("Overdue")}</td><td className={"r" + (late ? " late-t" : "")}>{late}</td></tr>
    </tbody></table>
  );
}

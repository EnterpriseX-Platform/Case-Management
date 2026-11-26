"use client";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useState } from "react";
import { api, Audit, Case, Screen, Version, Work } from "@/lib/api";
import { dt, loopLabel, overdue } from "@/lib/format";
import { useLoad } from "@/lib/useLoad";
import { has, useSession } from "@/lib/useSession";
import Dropdown from "@/components/Dropdown";
import ScreenRenderer from "@/components/ScreenRenderer";
import { Err, Page, Status } from "@/components/ui";

type CaseView = { case: Case; work: Work[]; audit: Audit[]; version: Version; children: Case[] };

const OVERVIEW: Screen = { key: "_case", name: "", kind: "case", layout: { rows: [
  { cols: [{ width: 4, widget: "caseInfo" }, { width: 8, widget: "flow" }] },
  { cols: [{ width: 6, widget: "subCases" }, { width: 6, widget: "history" }] },
] } };

export default function CasePage() {
  const { id } = useParams<{ id: string }>();
  const s = useSession();
  const { data, error, reload } = useLoad(() => api.get<CaseView>(`/api/cases/${id}`), [id]);
  const { data: screen } = useLoad(() => api.get<Screen>("/api/screens/case-overview").catch(() => OVERVIEW));
  const [e, setE] = useState<string>();
  const [moveTok, setMoveTok] = useState("");
  const [moveTo, setMoveTo] = useState("");
  const [reason, setReason] = useState("");
  if (!data) return <Page title="Case"><Err e={error} /></Page>;
  const c = data.case;
  const sup = has(s, "cm:supervisor");
  const nodes = Array.from(data.version.bpmn.matchAll(/<(userTask|manualTask|task)\s[^>]*id="([^"]+)"[^>]*name="([^"]*)"/g)).map((m) => ({ value: m[2], label: m[3] || m[2] }));
  const act = (p: Promise<unknown>) => p.then(reload).catch((x) => setE(x.message));
  const open = data.work.filter((w) => w.status === "open" || w.status === "claimed");

  return (
    <Page title={`${c.number} · ${c.title}`} actions={<Status s={c.status} />}>
      <Err e={e ?? c.instance.error} />
      <section className="card">
        <h3>Open work</h3>
        {open.length ? (
          <table className="tbl">
            <thead><tr><th>Step</th><th>Role</th><th>Assignee</th><th>Due</th><th /></tr></thead>
            <tbody>
              {open.map((w) => (
                <tr key={w.id} className={overdue(w.dueAt) ? "late" : ""}>
                  <td><Link href={`/work/${w.id}`}>{w.name}</Link>{loopLabel(w.input)}</td>
                  <td>{w.role}</td><td>{w.assignee || "-"}</td><td className="nw">{dt(w.dueAt)}</td>
                  <td className="nw">{sup && <button className="link" onClick={() => { setMoveTok(w.tokenId); setMoveTo(""); }}>Move step</button>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : <div className="muted">-</div>}
        {sup && moveTok && (
          <div className="row3 mt">
            <Dropdown placeholder="Target step" value={moveTo} onChange={setMoveTo} options={nodes} />
            <input placeholder="Reason" value={reason} onChange={(e) => setReason(e.target.value)} />
            <div>
              <button className="btn" disabled={!moveTo} onClick={() => act(api.post(`/api/cases/${c.id}/move`, { token: moveTok, to: moveTo, reason })).then(() => setMoveTok(""))}>Move</button>
              <button className="link" onClick={() => setMoveTok("")}>Cancel</button>
            </div>
          </div>
        )}
        {sup && c.status === "running" && (
          <div className="bar"><button className="btn-o danger" onClick={() => {
            const why = prompt("Reason for cancelling");
            if (why) act(api.post(`/api/cases/${c.id}/cancel`, { reason: why }));
          }}>Cancel case</button></div>
        )}
      </section>
      <ScreenRenderer screen={screen} ctx={{ c, version: data.version, audit: data.audit, children: data.children }} />
    </Page>
  );
}

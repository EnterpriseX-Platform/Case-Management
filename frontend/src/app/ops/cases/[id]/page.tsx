"use client";
// Process inspector: where a case stands in its flow and what is pending
// (work, jobs, timers, incidents), with operator actions: fix data, skip,
// move, retry and cancel. Every action asks for a reason and is audited.
import Link from "next/link";
import { useParams } from "next/navigation";
import { useState } from "react";
import { api, Case, Version, Work } from "@/lib/api";
import { dt } from "@/lib/format";
import { t } from "@/lib/i18n";
import { useLoad } from "@/lib/useLoad";
import Dropdown from "@/components/Dropdown";
import { Viewer } from "@/components/Bpmn";
import { useDialog } from "@/components/useDialog";
import { Err, Page, Status } from "@/components/ui";

type Token = { id: string; node: string; wait?: string; error?: string; miInner?: boolean };
type Insp = {
  case: Case & { instance: Case["instance"] & { tokens: Token[] | null } }; version: Version; work: Work[] | null;
  jobs: { id: string; node: string; status: string; attempts: number; maxAttempts: number; nextRunAt: string; lastError?: string }[] | null;
  timers: { id: string; node: string; dueAt: string }[] | null;
  incidents: { id: string; nodeName: string; message: string; status: string; createdAt: string }[] | null;
  events: { seq: number; at: string; type: string; node?: string; by?: string }[] | null; steps: Record<string, string>;
};
const WAIT: Record<string, string> = { user: "Waiting for a person", job: "Calling another system", timer: "Waiting for a time", message: "Waiting for a message",
  signal: "Waiting for a signal", incident: "Stopped by an error", join: "Waiting for parallel branches", scope: "Inside a sub-process", multi: "Per-item tasks", call: "Waiting for a sub-case", eventgw: "Waiting for the first event", condition: "Waiting for a condition" };

export default function Inspector() {
  const { id } = useParams<{ id: string }>();
  const { data: d, error, reload } = useLoad(() => api.get<Insp>(`/api/cases/${id}/inspect`), [id]);
  const dlg = useDialog();
  const [e, setE] = useState<string>();
  const [msg, setMsg] = useState<string>();
  const [moveTok, setMoveTok] = useState<string>();
  const [moveTo, setMoveTo] = useState("");
  const [edit, setEdit] = useState<{ key: string; value: string } | null>(null);
  if (!d) return <Page title={t("Case")}><Err e={error} /></Page>;
  const c = d.case;
  const tokens = ((c.instance.tokens ?? []) as Token[]).filter((x) => x.wait !== "scope" && x.wait !== "multi");
  const running = c.status === "running";
  const done = (m: string) => { setMsg(m); setE(undefined); reload(); };
  const fail = (x: any) => setE(x.message);
  const reason = (title: string) => dlg.prompt(title, t("Reason (kept in the audit log)"), "", { ok: t("Confirm") });
  const skip = async (tk: Token) => { const r = await reason(`${t("Skip")} · ${d.steps[tk.node]}`); if (r) api.post(`/api/cases/${id}/skip`, { token: tk.id, reason: r }).then(() => done(t("Step skipped"))).catch(fail); };
  const move = async () => { const r = await reason(`${t("Move to")} ${d.steps[moveTo]}`); if (r) api.post(`/api/cases/${id}/move`, { token: moveTok, to: moveTo, reason: r }).then(() => { setMoveTok(undefined); done(t("Moved")); }).catch(fail); };
  const retry = (incId: string) => api.post(`/api/incidents/${incId}/retry`, {}).then(() => done(t("Retried"))).catch(fail);
  const cancel = async () => { const r = await dlg.prompt(t("Cancel case"), t("Reason for cancelling"), "", { ok: t("Cancel case"), danger: true }); if (r) api.post(`/api/cases/${id}/cancel`, { reason: r }).then(() => done(t("Cancelled"))).catch(fail); };
  const saveVar = async () => {
    if (!edit) return;
    const r = await reason(`${t("Change")} ${edit.key}`);
    if (!r) return;
    let v: unknown = edit.value;
    try { v = JSON.parse(edit.value); } catch {}
    api.post(`/api/cases/${id}/vars`, { vars: { [edit.key]: v }, reason: r }).then(() => { setEdit(null); done(t("Saved")); }).catch(fail);
  };
  const nodes = Object.entries(d.steps).map(([k, v]) => ({ value: k, label: v }));
  const openIncidents = (d.incidents ?? []).filter((i) => i.status === "open");

  return (
    <Page crumb={{ href: "/ops/cases", label: t("Cases") }} title={c.title && c.title !== c.number ? `${c.number} · ${c.title}` : c.number} actions={<>
      <Status s={c.status} />
      <Link className="btn-o" href={`/cases/${c.id}`}>{t("Open case page")}</Link>
      {running && <button className="btn-o danger" onClick={cancel}>{t("Cancel case")}</button>}
    </>}>
      {dlg.node}
      <Err e={e ?? c.instance.error} />{msg && <div className="ok">{msg}</div>}
      <div className="cols">
        <section className="card">
          <h3>{t("Where the case is")}</h3>
          <Viewer xml={d.version.bpmn} active={tokens.map((x) => x.node)}
            done={Array.from(new Set((d.events ?? []).filter((x) => x.type === "leave" && x.node).map((x) => x.node!)))} />
          <table className="tbl mt">
            <thead><tr><th>{t("Step")}</th><th>{t("State")}</th><th /></tr></thead>
            <tbody>{tokens.length ? tokens.map((tk) => (
              <tr key={tk.id}>
                <td>{d.steps[tk.node] ?? tk.node}<div className="small muted dev-only mono">{tk.id} · {tk.node}</div></td>
                <td className={tk.wait === "incident" ? "late-t" : ""}>{t(WAIT[tk.wait ?? ""] ?? tk.wait ?? "-")}{tk.error && <div className="small">{tk.error}</div>}</td>
                <td className="nw">{running && <>
                  {tk.wait === "incident" && openIncidents[0] && <button className="btn-s" onClick={() => retry(openIncidents[0].id)}>{t("Retry")}</button>}
                  <button className="link" onClick={() => skip(tk)}>{t("Skip")}</button>
                  <button className="link" onClick={() => { setMoveTok(tk.id); setMoveTo(""); }}>{t("Move")}</button>
                </>}</td>
              </tr>
            )) : <tr><td colSpan={3} className="muted">-</td></tr>}</tbody>
          </table>
          {moveTok && (
            <div className="row3 mt">
              <Dropdown options={nodes} value={moveTo} onChange={setMoveTo} placeholder={t("Target step")} />
              <div><button className="btn-s" disabled={!moveTo} onClick={move}>{t("Move")}</button><button className="link" onClick={() => setMoveTok(undefined)}>{t("Cancel")}</button></div>
            </div>
          )}
        </section>
        <div>
          <section className="card">
            <h3>{t("Case data")}</h3>
            <table className="tbl"><tbody>
              {Object.entries(c.instance.vars).filter(([, v]) => typeof v !== "object" || Array.isArray(v)).map(([k, v]) => (
                <tr key={k}>
                  <td className="small">{k}</td>
                  <td>{edit?.key === k
                    ? <div className="row-url"><button className="btn-s" onClick={saveVar}>{t("Save")}</button><input value={edit.value} onChange={(x) => setEdit({ key: k, value: x.target.value })} /></div>
                    : <span>{Array.isArray(v) ? v.join(", ") : String(v)}</span>}</td>
                  <td>{running && edit?.key !== k && <button className="link" onClick={() => setEdit({ key: k, value: typeof v === "string" ? v : JSON.stringify(v) })}>{t("Edit")}</button>}</td>
                </tr>
              ))}
            </tbody></table>
          </section>
          <section className="card mt">
            <h3>{t("Pending")}</h3>
            <table className="tbl"><tbody>
              {(d.work ?? []).filter((w) => w.status === "open" || w.status === "claimed").map((w) => <tr key={w.id}><td>{w.name}</td><td className="small">{w.assignee || (w.queue ? `${t("Queue")} ${w.queue}` : w.role)}</td><td className="small nw">{dt(w.dueAt)}</td></tr>)}
              {(d.jobs ?? []).map((j) => <tr key={j.id}><td>{d.steps[j.node]}</td><td className="small">{t("Attempt")} {j.attempts + 1}/{j.maxAttempts}{j.lastError && <div className="late-t">{j.lastError}</div>}</td><td className="small nw">{dt(j.nextRunAt)}</td></tr>)}
              {(d.timers ?? []).map((x) => <tr key={x.id}><td>{d.steps[x.node]}</td><td className="small">{t("Timer")}</td><td className="small nw">{dt(x.dueAt)}</td></tr>)}
            </tbody></table>
          </section>
          <section className="card mt">
            <h3>{t("Recent events")}</h3>
            <table className="tbl"><tbody>
              {(d.events ?? []).slice(-15).reverse().map((x) => <tr key={x.seq}><td className="small nw">{dt(x.at)}</td><td className="small">{t(x.type)}</td><td className="small">{x.node ? d.steps[x.node] ?? x.node : ""}</td></tr>)}
            </tbody></table>
          </section>
        </div>
      </div>
    </Page>
  );
}

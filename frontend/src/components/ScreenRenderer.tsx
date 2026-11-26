"use client";
// Renders a low-code screen layout (rows of widgets) for a task or case.
import { t } from "@/lib/i18n";
import Link from "next/link";
import { Case, Screen, Widget, Audit, Version } from "@/lib/api";
import { dt, money, th } from "@/lib/format";
import { Viewer } from "./Bpmn";
import { Status } from "./ui";
import { KPIs, MyCases, MyWork, NewCase } from "./PortalWidgets";

export type Ctx = {
  form?: React.ReactNode;          // the rendered task form (or external screen)
  c?: Case;
  version?: Version;
  audit?: Audit[];
  children?: Case[];
  vars?: Record<string, unknown>;
};

export const DEFAULT_TASK: Screen = { key: "_default", name: "", kind: "task", layout: { rows: [
  { cols: [{ width: 8, widget: "form" }, { width: 4, widget: "caseInfo" }] },
  { cols: [{ width: 12, widget: "history" }] },
] } };

export const WIDGETS: { value: string; label: string }[] = [
  { value: "form", label: "Step form" }, { value: "caseInfo", label: t("Case details") },
  { value: "history", label: t("History") }, { value: "flow", label: "Flow diagram with current position" },
  { value: "subCases", label: t("Sub-cases") }, { value: "relatedCases", label: t("Related cases") },
  { value: "attachments", label: t("Attachments") }, { value: "comments", label: t("Comments") },
  { value: "iframe", label: "External page (iframe)" }, { value: "markdown", label: "Text" },
  { value: "worklist", label: "My work" }, { value: "myCases", label: "My cases" }, { value: "newCase", label: "New case" }, { value: "kpi", label: "Work figures" },
];

export default function ScreenRenderer({ screen, ctx }: { screen?: Screen; ctx: Ctx }) {
  const sc = screen ?? DEFAULT_TASK;
  return (
    <div className="screen">
      {sc.layout.rows.map((r, i) => (
        <div key={i} className="srow">
          {r.cols.map((c, j) => (
            <section key={j} className="scol" style={{ gridColumn: `span ${c.width || 12}` }}>
              <W w={c} ctx={ctx} />
            </section>
          ))}
        </div>
      ))}
    </div>
  );
}

function W({ w, ctx }: { w: Widget; ctx: Ctx }) {
  const c = ctx.c;
  switch (w.widget) {
    case "form":
      return <>{ctx.form}</>;
    case "caseInfo":
      return c ? (
        <div>
          <h3>{t("Case details")}</h3>
          <dl className="kv">
            <dt>{t("No.")}</dt><dd><Link href={`/cases/${c.id}`}>{c.number}</Link></dd>
            <dt>{t("Case")}</dt><dd>{c.title}</dd>
            <dt>{t("Fiscal year")}</dt><dd>{c.fiscalYear ? c.fiscalYear : "-"}</dd>
            <dt>{t("Status")}</dt><dd><Status s={c.status} /></dd>
            <dt>flow</dt><dd>{c.processKey} v{ctx.version?.no ?? "?"}</dd>
            <dt>{t("Initiator")}</dt><dd>{c.createdBy}</dd>
            <dt>{t("Created")}</dt><dd>{dt(c.createdAt)}</dd>
          </dl>
          <Vars vars={ctx.vars ?? c.instance.vars} />
        </div>
      ) : null;
    case "history":
      return (
        <div>
          <h3>{t("History")}</h3>
          <table className="tbl">
            <thead><tr><th>{t("Time")}</th><th>{t("By")}</th><th>{t("Event")}</th></tr></thead>
            <tbody>
              {(ctx.audit ?? []).map((a, i) => (
                <tr key={i}><td className="nw">{dt(a.at)}</td><td>{a.actor}</td><td>{auditText(a)}</td></tr>
              ))}
            </tbody>
          </table>
        </div>
      );
    case "flow":
      if (!c || !ctx.version) return null;
      return (
        <div>
          <h3>{t("Flow diagram")}</h3>
          <Viewer xml={ctx.version.bpmn}
            active={(c.instance.tokens ?? []).map((t) => t.node)}
            done={Array.from(new Set(c.instance.history.filter((h) => h.event === "leave").map((h) => h.node)))} />
        </div>
      );
    case "subCases":
      return (
        <div>
          <h3>{t("Sub-cases")}</h3>
          {(ctx.children ?? []).length ? (
            <table className="tbl"><tbody>
              {ctx.children!.map((k) => (
                <tr key={k.id}><td><Link href={`/cases/${k.id}`}>{k.number}</Link></td><td>{k.title}</td><td><Status s={k.status} /></td></tr>
              ))}
            </tbody></table>
          ) : <div className="muted">-</div>}
        </div>
      );
    case "relatedCases":
      return <div><h3>{t("Related cases")}</h3><div className="muted">{c?.parentId ? <Link href={`/cases/${c.parentId}`}>{t("Parent case")}</Link> : "-"}</div></div>;
    case "attachments": {
      const files = Object.entries(c?.instance.vars ?? {}).filter(([, v]) => Array.isArray(v) && v.every((x) => typeof x === "string" && /\.\w{2,5}$/.test(x)));
      return <div><h3>{t("Attachments")}</h3>{files.length ? <ul>{files.flatMap(([k, v]) => (v as string[]).map((f) => <li key={k + f}>{f}</li>))}</ul> : <div className="muted">-</div>}</div>;
    }
    case "comments":
      return <div><h3>{t("Comments")}</h3><div className="muted">{String(c?.instance.vars?.opinion ?? "-")}</div></div>;
    case "iframe":
      return w.url ? <iframe className="ext" src={w.url} /> : null;
    case "markdown":
      return <div className="md">{w.text}</div>;
    case "worklist":
      return <MyWork />;
    case "myCases":
      return <MyCases />;
    case "newCase":
      return <NewCase />;
    case "kpi":
      return <KPIs />;
    default:
      return null;
  }
}

function Vars({ vars }: { vars: Record<string, unknown> }) {
  const skip = new Set(["caseNumber", "initiator", "fiscalYear"]);
  const e = Object.entries(vars).filter(([k, v]) => !skip.has(k) && (typeof v !== "object" || Array.isArray(v)));
  if (!e.length) return null;
  return (
    <dl className="kv kv-vars">
      {e.map(([k, v]) => (
        <span key={k} style={{ display: "contents" }}><dt>{k}</dt><dd>{typeof v === "number" ? money(v) : Array.isArray(v) ? v.join(", ") : String(v)}</dd></span>
      ))}
    </dl>
  );
}

export function auditText(a: Audit) {
  const d = a.data ?? {};
  switch (a.action) {
    case "case.start": return `Opened case ${d.number ?? ""}`;
    case "work.complete": return `Completed step ${d.node ?? ""} Done`;
    case "work.claim": return "Claim";
    case "work.reassign": return `Assigned to ${d.to || d.role || "queue"}`;
    case "case.move": return `Moved to step ${d.to}${d.reason ? " — " + d.reason : ""}`;
    case "case.cancel": return `Cancel case${d.reason ? " — " + d.reason : ""}`;
    case "case.migrate": return "Migrated to a new flow version";
    case "step.skip": return `Skipped step ${d.node} (disabled in this version)`;
    default: return `${a.action} ${th(String(d.status ?? ""))}`.trim();
  }
}

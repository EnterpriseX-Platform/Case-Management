"use client";
// Renders a low-code screen layout (rows of widgets) for a task or case.
import Link from "next/link";
import { Case, Screen, Widget, Audit, Version } from "@/lib/api";
import { dt, money, th } from "@/lib/format";
import { Viewer } from "./Bpmn";
import { Status } from "./ui";

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
  { value: "form", label: "ฟอร์มของขั้นตอน" }, { value: "caseInfo", label: "ข้อมูลเรื่อง" },
  { value: "history", label: "ประวัติการดำเนินการ" }, { value: "flow", label: "แผนภาพ flow และตำแหน่งปัจจุบัน" },
  { value: "subCases", label: "เรื่องย่อย" }, { value: "relatedCases", label: "เรื่องที่เกี่ยวข้อง" },
  { value: "attachments", label: "เอกสารแนบ" }, { value: "comments", label: "ความเห็น" },
  { value: "iframe", label: "หน้าจอภายนอก (iframe)" }, { value: "markdown", label: "ข้อความ" },
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
          <h3>ข้อมูลเรื่อง</h3>
          <dl className="kv">
            <dt>เลขที่</dt><dd><Link href={`/cases/${c.id}`}>{c.number}</Link></dd>
            <dt>เรื่อง</dt><dd>{c.title}</dd>
            <dt>ปีงบประมาณ</dt><dd>{c.fiscalYear ? c.fiscalYear + 543 : "-"}</dd>
            <dt>สถานะ</dt><dd><Status s={c.status} /></dd>
            <dt>flow</dt><dd>{c.processKey} v{ctx.version?.no ?? "?"}</dd>
            <dt>ผู้ยื่น</dt><dd>{c.createdBy}</dd>
            <dt>วันที่ยื่น</dt><dd>{dt(c.createdAt)}</dd>
          </dl>
          <Vars vars={ctx.vars ?? c.instance.vars} />
        </div>
      ) : null;
    case "history":
      return (
        <div>
          <h3>ประวัติการดำเนินการ</h3>
          <table className="tbl">
            <thead><tr><th>เวลา</th><th>ผู้ดำเนินการ</th><th>รายการ</th></tr></thead>
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
          <h3>แผนภาพ flow</h3>
          <Viewer xml={ctx.version.bpmn}
            active={(c.instance.tokens ?? []).map((t) => t.node)}
            done={Array.from(new Set(c.instance.history.filter((h) => h.event === "leave").map((h) => h.node)))} />
        </div>
      );
    case "subCases":
      return (
        <div>
          <h3>เรื่องย่อย</h3>
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
      return <div><h3>เรื่องที่เกี่ยวข้อง</h3><div className="muted">{c?.parentId ? <Link href={`/cases/${c.parentId}`}>เรื่องหลัก</Link> : "-"}</div></div>;
    case "attachments": {
      const files = Object.entries(c?.instance.vars ?? {}).filter(([, v]) => Array.isArray(v) && v.every((x) => typeof x === "string" && /\.\w{2,5}$/.test(x)));
      return <div><h3>เอกสารแนบ</h3>{files.length ? <ul>{files.flatMap(([k, v]) => (v as string[]).map((f) => <li key={k + f}>{f}</li>))}</ul> : <div className="muted">-</div>}</div>;
    }
    case "comments":
      return <div><h3>ความเห็น</h3><div className="muted">{String(c?.instance.vars?.opinion ?? "-")}</div></div>;
    case "iframe":
      return w.url ? <iframe className="ext" src={w.url} /> : null;
    case "markdown":
      return <div className="md">{w.text}</div>;
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
    case "case.start": return `เปิดเรื่อง ${d.number ?? ""}`;
    case "work.complete": return `ดำเนินการขั้นตอน ${d.node ?? ""} เสร็จ`;
    case "work.claim": return "รับงาน";
    case "work.reassign": return `มอบหมายงานให้ ${d.to || d.role || "คิวงาน"}`;
    case "case.move": return `ย้ายไปขั้นตอน ${d.to}${d.reason ? " — " + d.reason : ""}`;
    case "case.cancel": return `ยกเลิกเรื่อง${d.reason ? " — " + d.reason : ""}`;
    case "case.migrate": return "ย้ายไปใช้ flow เวอร์ชันใหม่";
    case "step.skip": return `ข้ามขั้นตอน ${d.node} (ปิดใช้งานในเวอร์ชันนี้)`;
    default: return `${a.action} ${th(String(d.status ?? ""))}`.trim();
  }
}

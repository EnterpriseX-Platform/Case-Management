"use client";
// App Studio frame: one place for everything of an app version. Business
// mode shows the no-code designers; developer mode adds the BPMN editor,
// raw model and export.
import Link from "next/link";
import { useParams, usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { api, ProcessDef, Version } from "@/lib/api";
import { t } from "@/lib/i18n";
import { has, useSession } from "@/lib/useSession";
import { Err, Status } from "@/components/ui";

const SECTIONS: { k: string; label: string; dev?: boolean }[] = [
  { k: "lifecycle", label: "Lifecycle" }, { k: "data", label: "Data model" }, { k: "decisions", label: "Decision tables" },
  { k: "connectors", label: "Integrations" }, { k: "actions", label: "Actions" }, { k: "screens", label: "Step screens" },
  { k: "test", label: "Test run" }, { k: "flow", label: "BPMN flow", dev: true },
];
const NEXT: Record<string, { a: string; label: string; role: string }[]> = {
  draft: [{ a: "submit", label: "Submit for review", role: "cm:designer" }],
  in_review: [{ a: "approve", label: "Approve", role: "cm:approver" }, { a: "reject", label: "Reject", role: "cm:approver" }],
  approved: [{ a: "publish", label: "Publish", role: "cm:approver" }],
};

export default function StudioLayout({ children }: { children: React.ReactNode }) {
  const { key, vid } = useParams<{ key: string; vid: string }>();
  const path = usePathname();
  const r = useRouter();
  const s = useSession();
  const [p, setP] = useState<ProcessDef>();
  const [v, setV] = useState<Version>();
  const [e, setE] = useState<string>();
  const [dev, setDev] = useState(false);
  useEffect(() => { try { setDev(localStorage.getItem("cm.studio.dev") === "1"); } catch {} }, []);
  const load = () => {
    api.get<ProcessDef>(`/api/processes/${key}`).then(setP).catch(() => {});
    api.get<Version>(`/api/versions/${vid}`).then(setV).catch((x) => setE(x.message));
  };
  useEffect(load, [key, vid, path]);
  const toggle = () => { const d = !dev; setDev(d); try { localStorage.setItem("cm.studio.dev", d ? "1" : "0"); } catch {} };
  const section = path.split("/")[4];
  const transition = (a: string) => api.post<Version>(`/api/versions/${vid}/${a}`).then((x) => { setV(x); setE(undefined); }).catch((x) => setE(x.message));
  const copy = () => api.post<Version>(`/api/versions/${vid}/clone`, { fiscalYear: v?.fiscalYear }).then((x) => r.push(`/studio/${key}/${x.id}/${section}`)).catch((x) => setE(x.message));

  return (
    <div className="studio">
      <div className="studio-head">
        <div>
          <div className="small muted"><Link href="/studio">{t("App Studio")}</Link> / {key}</div>
          <h1>{p?.name ?? key} <span className="muted">v{v?.no}</span></h1>
        </div>
        <div className="actions">
          {v && <Status s={v.status} />}
          <button className={"toggle" + (dev ? " on" : "")} onClick={toggle} title={t("Developer mode")}>
            <span className="toggle-track"><span className="toggle-knob" /></span>{dev ? t("Developer") : t("Business")}
          </button>
          {(NEXT[v?.status ?? ""] ?? []).filter((x) => has(s, x.role)).map((x) => (
            <button key={x.a} className={x.a === "publish" ? "btn" : "btn-o"} onClick={() => transition(x.a)}>{t(x.label)}</button>
          ))}
          {v && v.status !== "draft" && has(s, "cm:designer") && <button className="btn-o" onClick={copy}>{t("New version")}</button>}
          {dev && <a className="btn-o" href={`/api/processes/${key}/export?version=${vid}`} onClick={async (ev) => {
            ev.preventDefault();
            const sess = JSON.parse(localStorage.getItem("cm.session") ?? "{}");
            const res = await fetch(`/api/processes/${key}/export?version=${vid}`, { headers: { "X-User": sess.id ?? "", "X-Roles": (sess.roles ?? []).join(",") } });
            const a = document.createElement("a"); a.href = URL.createObjectURL(await res.blob()); a.download = `${key}-v${v?.no}.cmpack`; a.click();
          }}>{t("Export")}</a>}
        </div>
      </div>
      <Err e={e} />
      <div className="studio-body">
        <nav className="studio-nav">
          {SECTIONS.filter((x) => !x.dev || dev).map((x) => (
            <Link key={x.k} href={`/studio/${key}/${vid}/${x.k}`} className={"side-a" + (section === x.k ? " on" : "")}>{t(x.label)}</Link>
          ))}
          <div className="side-h">{t("Versions")}</div>
          <Link className="side-a" href={`/designer/${key}`}>{t("All versions")}</Link>
        </nav>
        <div className="studio-main">{children}</div>
      </div>
    </div>
  );
}

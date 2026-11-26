"use client";
import { t } from "@/lib/i18n";
import { useDialog } from "@/components/useDialog";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { api, ProcessDef, Version } from "@/lib/api";
import { dt } from "@/lib/format";
import { useLoad } from "@/lib/useLoad";
import { has, useSession } from "@/lib/useSession";
import { EMPTY } from "@/components/Bpmn";
import Dropdown from "@/components/Dropdown";
import DropZone from "@/components/DropZone";
import { Err, Page, Status } from "@/components/ui";

type Diff = { added: Ch[]; removed: Ch[]; changed: Ch[] };
type Ch = { id: string; name: string; type: string; fields?: string[] };

const actions: Record<string, { a: string; label: string; role: string }[]> = {
  draft: [{ a: "submit", label: t("Submit for review"), role: "cm:designer" }],
  in_review: [{ a: "approve", label: t("Approve"), role: "cm:approver" }, { a: "reject", label: t("Reject"), role: "cm:approver" }],
  approved: [{ a: "publish", label: t("Publish"), role: "cm:approver" }],
  published: [{ a: "deprecate", label: t("Deprecate"), role: "cm:approver" }],
  deprecated: [{ a: "archive", label: t("Archive"), role: "cm:admin" }],
};

export default function Versions() {
  const { key, vid } = useParams<{ key: string; vid?: string }>();
  const inStudio = !!vid;
  const router = useRouter();
  useEffect(() => {
    if (inStudio) return;
    api.get<Version[]>(`/api/processes/${key}/versions`).then((vs) => vs?.[0] && router.replace(`/studio/${key}/${vs[0].id}/versions`)).catch(() => {});
  }, [inStudio, key, router]);
  const s = useSession();
  const r = useRouter();
  const { data: p } = useLoad(() => api.get<ProcessDef>(`/api/processes/${key}`), [key]);
  const { data, error, reload } = useLoad(() => api.get<Version[]>(`/api/processes/${key}/versions`), [key]);
  const [e, setE] = useState<string>();
  const dlg = useDialog();
  const [warn, setWarn] = useState<string[]>([]);
  const [a, setA] = useState(""), [b, setB] = useState("");
  const [diff, setDiff] = useState<Diff>();
  const act = (p: Promise<unknown>) => p.then(reload).catch((x) => setE(x.message));
  const opts = (data ?? []).map((v) => ({ value: v.id, label: `v${v.no} · ${v.fiscalYear ? v.fiscalYear : "-"} · ${v.status}` }));

  const newDraft = async (xml: string, note: string) => {
    try {
      const res = await api.post<{ version: Version; warnings: string[] }>(`/api/processes/${key}/versions`, { bpmn: xml, note });
      setWarn(res.warnings ?? []);
      r.push(`/studio/${key}/${res.version.id}/lifecycle`);
    } catch (x: any) { setE(x.message); }
  };

  return (
    <Page title={inStudio ? t("Versions") : p?.name ?? key} actions={has(s, "cm:designer") && <button className="btn" onClick={() => newDraft(EMPTY, "")}>{t("New version")}</button>}>
      {dlg.node}
      <Err e={error ?? e} />
      {warn.map((w) => <div key={w} className="warn">{w}</div>)}
      <table className="tbl">
        <thead><tr><th>{t("Version")}</th><th>{t("Fiscal year")}</th><th>{t("Status")}</th><th>{t("Note")}</th><th>{t("Author")}</th><th>{t("Approver")}</th><th>{t("Published at")}</th><th /></tr></thead>
        <tbody>
          {(data ?? []).map((v) => (
            <tr key={v.id}>
              <td><Link href={`/studio/${key}/${v.id}/lifecycle`}>v{v.no}</Link></td>
              <td>{v.fiscalYear ? v.fiscalYear : "-"}</td>
              <td><Status s={v.status} /></td>
              <td>{v.note}</td><td>{v.createdBy}</td><td>{v.approvedBy ?? "-"}</td><td className="nw">{dt(v.publishedAt)}</td>
              <td className="nw">
                                <Link className="link" href={`/studio/${key}/${v.id}/screens`}>{t("Step screens")}</Link>
                {(actions[v.status] ?? []).filter((x) => has(s, x.role)).map((x) => (
                  <button key={x.a} className="link" onClick={() => act(api.post(`/api/versions/${v.id}/${x.a}`))}>{x.label}</button>
                ))}
                {has(s, "cm:designer") && (
                  <button className="link" onClick={async () => {
                    const y = await dlg.prompt(t("New version"), t("Fiscal year of the new version"), v.fiscalYear ? String(v.fiscalYear + 1) : "", { ok: t("Create") });
                    if (y) act(api.post<Version>(`/api/versions/${v.id}/clone`, { fiscalYear: Number(y) }).then((n) => r.push(`/studio/${key}/${n.id}/lifecycle`)));
                  }}>{t("Copy for next year")}</button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      {has(s, "cm:designer") && (
        <section className="card mt">
          <h3>{t("Import BPMN as a new version")}</h3>
          <DropZone accept=".bpmn,.xml" onFiles={async (fs) => newDraft(await fs[0].text(), "Imported from " + fs[0].name)} />
        </section>
      )}

      <section className="card mt">
        <h3>{t("Compare versions")}</h3>
        <div className="row3">
          <Dropdown placeholder={t("From")} value={a} onChange={setA} options={opts} />
          <Dropdown placeholder={t("To")} value={b} onChange={setB} options={opts} />
          <div><button className="btn-o" disabled={!a || !b} onClick={() => api.get<Diff>(`/api/versions/${a}/diff?to=${b}`).then(setDiff).catch((x) => setE(x.message))}>{t("Compare")}</button></div>
        </div>
        {diff && (
          <table className="tbl mt">
            <thead><tr><th>{t("Change")}</th><th>{t("Step")}</th><th>{t("Type")}</th><th>{t("Details")}</th></tr></thead>
            <tbody>
              {diff.added.map((c) => <tr key={"a" + c.id} className="d-add"><td>{t("Added")}</td><td>{c.name || c.id}</td><td>{c.type}</td><td /></tr>)}
              {diff.removed.map((c) => <tr key={"r" + c.id} className="d-del"><td>{t("Remove")}</td><td>{c.name || c.id}</td><td>{c.type}</td><td /></tr>)}
              {diff.changed.map((c) => <tr key={"c" + c.id} className="d-chg"><td>{t("Changed")}</td><td>{c.name || c.id}</td><td>{c.type}</td><td>{c.fields?.join(", ")}</td></tr>)}
            </tbody>
          </table>
        )}
      </section>
    </Page>
  );
}

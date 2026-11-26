"use client";
import { useStudioRedirect } from "@/components/StudioRedirect";
import { t } from "@/lib/i18n";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useRef, useState } from "react";
import { api, Version } from "@/lib/api";
import { useLoad } from "@/lib/useLoad";
import { Modeler, ModelerHandle } from "@/components/Bpmn";
import { Err, Field, Page, Status } from "@/components/ui";

export default function FlowEditor() {
  const prm = useParams<{ id?: string; vid?: string }>();
  const id = (prm.id ?? prm.vid) as string; // also mounted inside the App Studio
  const inStudio = !!prm.vid;
  useStudioRedirect(id, inStudio, "flow");
  const { data: v, error, setData } = useLoad(() => api.get<Version>(`/api/versions/${id}`), [id]);
  const m = useRef<ModelerHandle>(null);
  const [e, setE] = useState<string>();
  const [msg, setMsg] = useState<string>();
  const [warn, setWarn] = useState<string[]>([]);
  const [sel, setSel] = useState<{ id?: string; name?: string; type?: string }>({});
  const [fy, setFy] = useState("");
  if (!v) return <Page title="flow"><Err e={error} /></Page>;
  const draft = v.status === "draft";

  const save = async () => {
    setE(undefined); setMsg(undefined);
    try {
      const xml = await m.current!.getXML();
      const r = await api.put<{ version: Version; warnings: string[] }>(`/api/versions/${id}`, { bpmn: xml, fiscalYear: fy ? Number(fy) : 0 });
      setWarn(r.warnings ?? []);
      setMsg("Saved");
      setData(r.version);
    } catch (x: any) { setE(x.message); }
  };
  const download = async () => {
    const xml = await m.current!.getXML();
    const a = document.createElement("a");
    a.href = URL.createObjectURL(new Blob([xml], { type: "application/xml" }));
    a.download = `${v.processKey}-v${v.no}.bpmn`;
    a.click();
  };

  return (
    <Page title={t("BPMN flow")} actions={<>
      <button className="btn-o" onClick={download}>{t("Export BPMN")}</button>
      {draft && <button className="btn" onClick={save}>{t("Save")}</button>}
    </>}>
      <Err e={e} />
      {msg && <div className="ok">{msg}</div>}
      {warn.map((w) => <div key={w} className="warn">{w}</div>)}
      {!draft && <div className="warn">This version {v.status === "published" ? "is in use" : "is not a draft"} and cannot be edited. Copy it to a new version.</div>}
      {draft && (
        <div className="row3">
          <Field label={t("Fiscal year")}><input inputMode="numeric" placeholder={v.fiscalYear ? String(v.fiscalYear) : ""} value={fy} onChange={(x) => setFy(x.target.value)} /></Field>
          <Field label={t("Selected element")}><div className="display">{sel.id ? `${sel.name || "(unnamed)"} · ${sel.type} · ${sel.id}` : "-"}</div></Field>
        </div>
      )}
      <Modeler ref={m} xml={v.bpmn} readOnly={!draft} onSelect={(id, name, type) => setSel({ id, name, type })} />
    </Page>
  );
}

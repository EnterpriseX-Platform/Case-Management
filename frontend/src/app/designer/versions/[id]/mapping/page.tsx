"use client";
// Screen mapping: choose, per flow step, a low-code form + screen, an
// external screen, or no screen (API only); plus role, SLA and step toggle.
import { t } from "@/lib/i18n";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useEffect, useState } from "react";
import { api, Binding, Form, Screen, Step, Version } from "@/lib/api";
import { useLoad } from "@/lib/useLoad";
import Dropdown from "@/components/Dropdown";
import { Viewer } from "@/components/Bpmn";
import { Err, Page, Status } from "@/components/ui";

const UI = [
  { value: "form", label: "Built-in screen (low-code form)" },
  { value: "external", label: "External screen" },
  { value: "none", label: "No screen (API only)" },
];
const SLA = ["PT4H", "P1D", "P3D", "P7D", "P14D", "P30D", "P60D"].map((x) => ({ value: x, label: x }));

export default function Mapping() {
  const { id } = useParams<{ id: string }>();
  const { data: v } = useLoad(() => api.get<Version>(`/api/versions/${id}`), [id]);
  const { data: steps, error } = useLoad(() => api.get<Step[]>(`/api/versions/${id}/steps`), [id]);
  const { data: forms } = useLoad(() => api.get<Form[]>("/api/forms"));
  const { data: screens } = useLoad(() => api.get<Screen[]>("/api/screens"));
  const [rows, setRows] = useState<Step[]>([]);
  const [e, setE] = useState<string>();
  const [msg, setMsg] = useState<string>();
  const [focus, setFocus] = useState<string>();
  useEffect(() => { if (steps) setRows(steps); }, [steps]);

  const set = (i: number, b: Partial<Binding>) => setRows(rows.map((r, j) => (j === i ? { ...r, binding: { ...r.binding, ...b } } : r)));
  const save = () =>
    api.put<Step[]>(`/api/versions/${id}/bindings`, rows.map((r) => ({ ...r.binding, nodeId: r.nodeId })))
      .then((s) => { setRows(s); setMsg("Saved"); setE(undefined); }).catch((x) => { setE(x.message); setMsg(undefined); });
  const fo = (forms ?? []).map((f) => ({ value: f.key, label: f.name, hint: f.key }));
  const so = (screens ?? []).filter((s) => s.kind === "task").map((s) => ({ value: s.key, label: s.name, hint: s.key }));

  return (
    <Page title={`Step screens · ${v?.processKey ?? ""} v${v?.no ?? ""}`} actions={<>
      {v && <Status s={v.status} />}
      <Link className="btn-o" href={`/designer/versions/${id}`}>{t("Edit flow")}</Link>
      <button className="btn" onClick={save}>{t("Save")}</button>
    </>}>
      <Err e={error ?? e} />
      {msg && <div className="ok">{msg}</div>}
      {v && v.status !== "draft" && <div className="warn">A published version only allows changes to role, SLA and step toggle</div>}
      {v && <Viewer xml={v.bpmn} active={focus ? [focus] : []} />}
      <table className="tbl map">
        <thead><tr><th>{t("Step")}</th><th>{t("Presentation")}</th><th>Forms / URL</th><th>{t("Screens")}</th><th>{t("Role")}</th><th>SLA</th><th>{t("Enabled")}</th><th>{t("Condition")}</th></tr></thead>
        <tbody>
          {rows.map((r, i) => {
            const b = r.binding;
            const on = b.enabled !== false;
            return (
              <tr key={r.nodeId} onMouseEnter={() => setFocus(r.nodeId)} className={on ? "" : "off"}>
                <td><div>{r.name || r.nodeId}</div><div className="muted small">{r.type} · {r.nodeId}</div></td>
                <td>{r.human ? <Dropdown options={UI} value={b.ui} onChange={(x) => set(i, { ui: x })} /> : <span className="muted">{t("Automatic")}</span>}</td>
                <td>
                  {r.human && b.ui === "form" && <Dropdown options={fo} value={b.formKey} onChange={(x) => set(i, { formKey: x })} placeholder={t("Select form")} />}
                  {r.human && b.ui === "external" && (
                    <div>
                      <input placeholder="https://" value={b.url ?? ""} onChange={(x) => set(i, { url: x.target.value })} />
                      <button type="button" className={"check sm" + (b.embed ? " on" : "")} onClick={() => set(i, { embed: !b.embed })}>{b.embed ? "✓" : ""}</button>
                      <span className="small"> {t("Embed in page")}</span>
                    </div>
                  )}
                </td>
                <td>{r.human && b.ui !== "none" && <Dropdown options={so} value={b.screenKey} onChange={(x) => set(i, { screenKey: x })} placeholder={t("Default")} />}</td>
                <td>{r.human && <input value={b.role ?? ""} placeholder={r.lane} onChange={(x) => set(i, { role: x.target.value })} />}</td>
                <td>{r.human && <Dropdown options={SLA} value={b.sla} allowCustom onChange={(x) => set(i, { sla: x })} placeholder="-" />}</td>
                <td>{r.human && <button type="button" className={"check" + (on ? " on" : "")} onClick={() => set(i, { enabled: !on })}>{on ? "✓" : ""}</button>}</td>
                <td>{r.human && <input placeholder="e.g. amount > 1000000" value={b.condition ?? ""} onChange={(x) => set(i, { condition: x.target.value })} />}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </Page>
  );
}

"use client";
// Decision tables: spreadsheet editor (input columns × output columns) with
// an inline tester that highlights the matching rows.
import { useParams } from "next/navigation";
import { useEffect, useState } from "react";
import { api, CaseModel, DecisionTable, Version } from "@/lib/api";
import { t } from "@/lib/i18n";
import { useLoad } from "@/lib/useLoad";
import Dropdown from "@/components/Dropdown";
import { Err, Field } from "@/components/ui";

const slug = (s: string) => s.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "table";

export default function Decisions() {
  const { vid } = useParams<{ vid: string }>();
  const { data: v } = useLoad(() => api.get<Version>(`/api/versions/${vid}`), [vid]);
  const { data: loaded } = useLoad(() => api.get<CaseModel>(`/api/versions/${vid}/model`), [vid]);
  const [m, setM] = useState<CaseModel>();
  const [cur, setCur] = useState(0);
  const [e, setE] = useState<string>();
  const [msg, setMsg] = useState<string>();
  const [sample, setSample] = useState<Record<string, string>>({});
  const [res, setRes] = useState<{ output: Record<string, unknown>; rows: number[] }>();
  useEffect(() => { if (loaded) setM({ ...loaded, decisions: loaded.decisions ?? [] }); }, [loaded]);
  if (!m) return <Err e={e} />;
  const draft = v?.status === "draft";
  const list = m.decisions ?? [];
  const dt = list[cur];
  const setDT = (p: Partial<DecisionTable>) => setM({ ...m, decisions: list.map((d, i) => (i === cur ? { ...d, ...p } : d)) });
  const fields = m.data.map((f) => ({ value: f.key, label: `${f.label} (${f.key})` }));
  const save = () => api.put<CaseModel>(`/api/versions/${vid}/model`, m).then((x) => { setM({ ...x, decisions: x.decisions ?? [] }); setMsg(t("Saved")); setE(undefined); }).catch((x) => { setE(x.message); setMsg(undefined); });
  const test = async () => {
    const vars: Record<string, unknown> = {};
    for (const [k, val] of Object.entries(sample)) vars[k] = val !== "" && !isNaN(Number(val)) ? Number(val) : val === "true" ? true : val === "false" ? false : val;
    try {
      if (draft) await api.put(`/api/versions/${vid}/model`, m); // test what is on screen
      setRes(await api.post(`/api/versions/${vid}/decisions/${dt.key}/test`, vars)); setE(undefined); } catch (x: any) { setE(x.message); }
  };
  const cell = (ri: number, side: "when" | "then", ci: number, val: string) =>
    setDT({ rows: dt.rows.map((r, i) => i !== ri ? r : { ...r, [side]: Object.assign([...(r[side] ?? [])], { [ci]: val }) }) });

  return (
    <div>
      <div className="page-head">
        <h1>{t("Decision tables")}</h1>
        <div className="actions">
          {draft && <button className="btn-o" onClick={() => {
            const key = slug(`table-${list.length + 1}`);
            setM({ ...m, decisions: [...list, { key, name: t("New decision"), inputs: [{ key: m.data[0]?.key ?? "amount", label: m.data[0]?.label ?? "Amount" }], outputs: [{ key: "result", label: "Result" }], rows: [{ when: [""], then: [""] }] }] });
            setCur(list.length);
          }}>{t("New table")}</button>}
          {draft && <button className="btn" onClick={save}>{t("Save")}</button>}
        </div>
      </div>
      <Err e={e} />{msg && <div className="ok">{msg}</div>}
      {list.length > 1 && <div className="tabs">{list.map((d, i) => <button key={d.key} className={i === cur ? "on" : ""} onClick={() => { setCur(i); setRes(undefined); }}>{d.name}</button>)}</div>}
      {!dt ? <div className="empty muted">—</div> : (
        <>
          <div className="row3">
            <Field label={t("Name")}><input value={dt.name} disabled={!draft} onChange={(x) => setDT({ name: x.target.value })} /></Field>
            <Field label={t("Key")}><input className="mono" value={dt.key} disabled={!draft} onChange={(x) => setDT({ key: slug(x.target.value) })} /></Field>
            <Field label={t("Hit policy")}><Dropdown options={[{ value: "first", label: t("First match") }, { value: "collect", label: t("Collect all matches") }]} value={dt.hit ?? "first"} disabled={!draft} onChange={(x) => setDT({ hit: x })} /></Field>
          </div>
          <section className="card">
            <table className="tbl grid-sheet">
              <thead>
                <tr><th>#</th>
                  {dt.inputs.map((c, ci) => (
                    <th key={"i" + ci}>
                      <Dropdown options={fields} allowCustom value={c.key} disabled={!draft}
                        onChange={(k) => setDT({ inputs: dt.inputs.map((x, j) => (j === ci ? { key: k, label: m.data.find((f) => f.key === k)?.label ?? k } : x)) })} />
                    </th>
                  ))}
                  {draft && <th><button className="link" onClick={() => setDT({ inputs: [...dt.inputs, { key: "", label: "" }] })}>+ {t("Input")}</button></th>}
                  {dt.outputs.map((c, ci) => (
                    <th key={"o" + ci} className="out">
                      <input value={c.key} disabled={!draft} placeholder={t("field")} onChange={(x) => setDT({ outputs: dt.outputs.map((y, j) => (j === ci ? { key: x.target.value, label: x.target.value } : y)) })} />
                    </th>
                  ))}
                  {draft && <th className="out"><button className="link" onClick={() => setDT({ outputs: [...dt.outputs, { key: "", label: "" }] })}>+ {t("Output")}</button></th>}
                  <th />
                </tr>
              </thead>
              <tbody>
                {dt.rows.map((r, ri) => (
                  <tr key={ri} className={res?.rows.includes(ri + 1) ? "hit" : ""}>
                    <td className="small">{ri + 1}</td>
                    {dt.inputs.map((_, ci) => <td key={"i" + ci}><input value={r.when[ci] ?? ""} placeholder="-" disabled={!draft} onChange={(x) => cell(ri, "when", ci, x.target.value)} /></td>)}
                    {draft && <td />}
                    {dt.outputs.map((_, ci) => <td key={"o" + ci}><input value={r.then[ci] ?? ""} disabled={!draft} onChange={(x) => cell(ri, "then", ci, x.target.value)} /></td>)}
                    {draft && <td />}
                    <td className="nw">{draft && <button className="link danger" onClick={() => setDT({ rows: dt.rows.filter((_, i) => i !== ri) })}>{t("Remove")}</button>}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            {draft && <div className="bar"><button className="btn-s" onClick={() => setDT({ rows: [...dt.rows, { when: dt.inputs.map(() => ""), then: dt.outputs.map(() => "") }] })}>{t("Add row")}</button></div>}
            <div className="small muted mt mono">&gt;= 1000000 · 100000..999999 · in ["it","vehicle"] · == "urgent" · true · -</div>
          </section>
          <section className="card mt">
            <h3>{t("Test")}</h3>
            <div className="row3">
              {Array.from(new Set(dt.inputs.map((c) => c.key.split(/[^A-Za-z0-9_]/)[0]).filter(Boolean))).map((k) => (
                <Field key={k} label={k}><input value={sample[k] ?? ""} onChange={(x) => setSample({ ...sample, [k]: x.target.value })} /></Field>
              ))}
            </div>
            <div className="bar"><button className="btn-o" onClick={test}>{t("Run test")}</button></div>
            {res && <pre className="code mt">{JSON.stringify(res, null, 2)}</pre>}
          </section>
        </>
      )}
    </div>
  );
}

"use client";
// Integrations: REST connectors with request / response mapping and a live
// test call. Credentials are referenced as ${secret:NAME} (server env), never stored.
import { useParams } from "next/navigation";
import { useEffect, useState } from "react";
import { api, CaseModel, Connector, Version } from "@/lib/api";
import { t } from "@/lib/i18n";
import { useLoad } from "@/lib/useLoad";
import Dropdown from "@/components/Dropdown";
import { Err, Field } from "@/components/ui";

const slug = (s: string) => s.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "api";
const METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE"].map((x) => ({ value: x, label: x }));
type Res = { status: number; request: unknown; response: unknown; output: Record<string, unknown>; millis: number; error?: string };

function KV({ value, onChange, left, right, disabled }: { value?: Record<string, string>; onChange: (v: Record<string, string>) => void; left: string; right: string; disabled?: boolean }) {
  const rows = Object.entries(value ?? {});
  const set = (i: number, k: string, v: string) => {
    const n = rows.slice(); n[i] = [k, v]; onChange(Object.fromEntries(n.filter(([a]) => a !== "" || true)));
  };
  return (
    <table className="tbl grid-sheet">
      <thead><tr><th>{left}</th><th>{right}</th><th /></tr></thead>
      <tbody>
        {rows.map(([k, v], i) => (
          <tr key={i}>
            <td><input value={k} disabled={disabled} onChange={(x) => set(i, x.target.value, v)} /></td>
            <td><input value={v} disabled={disabled} onChange={(x) => set(i, k, x.target.value)} /></td>
            <td>{!disabled && <button className="link danger" onClick={() => onChange(Object.fromEntries(rows.filter((_, j) => j !== i)))}>{t("Remove")}</button>}</td>
          </tr>
        ))}
        {!disabled && <tr><td colSpan={3}><button className="link" onClick={() => onChange({ ...Object.fromEntries(rows), "": "" })}>+ {t("Add")}</button></td></tr>}
      </tbody>
    </table>
  );
}

export default function Connectors() {
  const { vid } = useParams<{ vid: string }>();
  const { data: v } = useLoad(() => api.get<Version>(`/api/versions/${vid}`), [vid]);
  const { data: loaded } = useLoad(() => api.get<CaseModel>(`/api/versions/${vid}/model`), [vid]);
  const [m, setM] = useState<CaseModel>();
  const [cur, setCur] = useState(0);
  const [e, setE] = useState<string>();
  const [msg, setMsg] = useState<string>();
  const [vars, setVars] = useState('{\n  "amount": 1000,\n  "caseNumber": "TEST-1"\n}');
  const [res, setRes] = useState<Res>();
  useEffect(() => { if (loaded) setM({ ...loaded, connectors: loaded.connectors ?? [] }); }, [loaded]);
  if (!m) return <Err e={e} />;
  const draft = v?.status === "draft";
  const list = m.connectors ?? [];
  const c = list[cur];
  const setC = (p: Partial<Connector>) => setM({ ...m, connectors: list.map((x, i) => (i === cur ? { ...x, ...p } : x)) });
  const save = () => api.put<CaseModel>(`/api/versions/${vid}/model`, m).then((x) => { setM({ ...x, connectors: x.connectors ?? [] }); setMsg(t("Saved")); setE(undefined); }).catch((x) => { setE(x.message); setMsg(undefined); });
  const test = async () => {
    try {
      const r = await api.post<Res>("/api/connectors/test", { connector: c, vars: JSON.parse(vars) });
      setRes(r); setE(undefined);
    } catch (x: any) { setE(x.message); }
  };
  return (
    <div>
      <div className="page-head">
        <h1>{t("Integrations")}</h1>
        <div className="actions">
          {draft && <button className="btn-o" onClick={() => { setM({ ...m, connectors: [...list, { key: slug(`api-${list.length + 1}`), name: t("New integration"), method: "POST", url: "https://", body: {}, response: {} }] }); setCur(list.length); }}>{t("New integration")}</button>}
          {draft && <button className="btn" onClick={save}>{t("Save")}</button>}
        </div>
      </div>
      <Err e={e} />{msg && <div className="ok">{msg}</div>}
      {list.length > 1 && <div className="tabs">{list.map((x, i) => <button key={x.key} className={i === cur ? "on" : ""} onClick={() => { setCur(i); setRes(undefined); }}>{x.name}</button>)}</div>}
      {!c ? <div className="empty muted">—</div> : (
        <div className="studio-split">
          <div>
            <section className="card">
              <div className="row3">
                <Field label={t("Name")}><input value={c.name} disabled={!draft} onChange={(x) => setC({ name: x.target.value })} /></Field>
                <Field label={t("Key")}><input className="mono" value={c.key} disabled={!draft} onChange={(x) => setC({ key: slug(x.target.value) })} /></Field>
                <Field label={t("Retries")}><input inputMode="numeric" value={c.retries ?? ""} placeholder="5" disabled={!draft} onChange={(x) => setC({ retries: Number(x.target.value) || undefined })} /></Field>
              </div>
              <div className="row-url">
                <Dropdown options={METHODS} value={c.method} disabled={!draft} onChange={(x) => setC({ method: x })} />
                <input className="mono" value={c.url} disabled={!draft} onChange={(x) => setC({ url: x.target.value })} placeholder="https://api.example.com/orders/${caseNumber}" />
              </div>
            </section>
            <section className="card mt"><h3>{t("Headers")}</h3><KV value={c.headers} onChange={(h) => setC({ headers: h })} left={t("Header")} right={t("Value")} disabled={!draft} />
              <div className="small muted mono">Authorization = ${"{"}secret:ERP_TOKEN{"}"}</div></section>
            <section className="card mt"><h3>{t("Request body")}</h3><KV value={c.body} onChange={(b) => setC({ body: b })} left={t("JSON key")} right={t("Value from case")} disabled={!draft} /></section>
            <section className="card mt"><h3>{t("Response mapping")}</h3><KV value={c.response} onChange={(b) => setC({ response: b })} left={t("Case field")} right={t("Response path")} disabled={!draft} /></section>
          </div>
          <aside className="panel">
            <h3>{t("Test call")}</h3>
            <Field label={t("Sample case data")}><textarea className="mono" rows={6} value={vars} onChange={(x) => setVars(x.target.value)} /></Field>
            <button className="btn-o" onClick={test}>{t("Send")}</button>
            {res && <>
              <div className={res.error ? "err mt" : "ok mt"}>HTTP {res.status || "—"} · {res.millis} ms{res.error ? ` · ${res.error}` : ""}</div>
              <div className="fld-l">{t("Mapped to case")}</div><pre className="code">{JSON.stringify(res.output, null, 2)}</pre>
              <div className="fld-l mt">{t("Response")}</div><pre className="code">{JSON.stringify(res.response, null, 2)}</pre>
              <div className="fld-l mt">{t("Request")}</div><pre className="code">{JSON.stringify(res.request, null, 2)}</pre>
            </>}
          </aside>
        </div>
      )}
    </div>
  );
}

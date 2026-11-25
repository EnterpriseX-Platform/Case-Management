"use client";
import { useParams, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { api, Audit, Case, Form, Screen, Version, Work } from "@/lib/api";
import { useLoad } from "@/lib/useLoad";
import { useSession } from "@/lib/useSession";
import FormRenderer, { validate } from "@/components/FormRenderer";
import ScreenRenderer from "@/components/ScreenRenderer";
import { Err, Page, Status } from "@/components/ui";

type WorkView = { work: Work; form?: Form; screen?: Screen; vars?: Record<string, any>; callback?: string };
type CaseView = { case: Case; audit: Audit[]; version: Version; children: Case[] };

export default function TaskPage() {
  const { id } = useParams<{ id: string }>();
  const r = useRouter();
  const s = useSession();
  const { data, error, reload } = useLoad(() => api.get<WorkView>(`/api/work/${id}`), [id]);
  const [cv, setCv] = useState<CaseView>();
  const [d, setD] = useState<Record<string, any>>({});
  const [errs, setErrs] = useState<Record<string, string>>({});
  const [e, setE] = useState<string>();
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!data) return;
    api.get<CaseView>(`/api/cases/${data.work.caseId}`).then(setCv).catch(() => {});
    // prefill with current case values for fields of this form
    const pre: Record<string, any> = { ...(data.work.input ?? {}) };
    for (const f of data.form?.schema.fields ?? []) if (data.vars && f.key in data.vars && !(f.key in pre)) pre[f.key] = data.vars[f.key];
    setD(pre);
  }, [data]);

  if (!data) return <Page title="งาน"><Err e={error} /></Page>;
  const w = data.work;
  const done = w.status === "done" || w.status === "cancelled";
  const mine = w.assignee === s?.id;
  const fields = data.form?.schema.fields ?? [];

  const submit = async () => {
    const v = validate(fields, d);
    setErrs(v);
    if (Object.keys(v).length) return;
    setBusy(true);
    try {
      if (!mine) await api.post(`/api/work/${w.id}/claim`);
      await api.post(`/api/work/${w.id}/complete`, d);
      r.push("/work");
    } catch (x: any) {
      setE(x.message);
    } finally {
      setBusy(false);
    }
  };

  let body: React.ReactNode;
  if (w.ui === "external") {
    const src = w.url ? `${w.url}${w.url.includes("?") ? "&" : "?"}callback=${encodeURIComponent(data.callback ?? "")}&case=${w.caseNumber}` : "";
    body = (
      <div>
        <h3>{w.name}</h3>
        {w.embed ? <iframe className="ext" src={src} />
          : <a className="btn" href={src} target="_blank" rel="noreferrer">เปิดหน้าจอระบบภายนอก</a>}
        <div className="muted small">ขั้นตอนนี้จะเสร็จเมื่อระบบภายนอกส่งผลกลับ</div>
        {!done && <div className="bar"><button className="btn-s" onClick={reload}>ตรวจสอบสถานะ</button></div>}
      </div>
    );
  } else {
    body = (
      <div>
        <h3>{w.name}</h3>
        {data.form ? <FormRenderer fields={fields} data={d} onChange={setD} readOnly={done} errors={errs} />
          : <div className="muted">ขั้นตอนนี้ไม่มีฟอร์ม</div>}
        <Err e={e} />
        {!done && (
          <div className="bar">
            <button className="btn" disabled={busy} onClick={submit}>{w.ui === "none" ? "ยืนยันดำเนินการแล้ว" : "บันทึกและส่งต่อ"}</button>
            {!mine && !w.assignee && <button className="btn-o" onClick={() => api.post(`/api/work/${w.id}/claim`).then(reload).catch((x) => setE(x.message))}>รับงาน</button>}
          </div>
        )}
      </div>
    );
  }

  return (
    <Page title={`${w.caseNumber} · ${w.name}`} actions={<Status s={w.status} />}>
      <ScreenRenderer screen={data.screen ?? undefined}
        ctx={{ form: body, c: cv?.case, version: cv?.version, audit: cv?.audit, children: cv?.children, vars: data.vars }} />
    </Page>
  );
}

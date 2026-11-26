"use client";
// A work item done right on the case page: its form and the submit button.
import Link from "next/link";
import { useEffect, useState } from "react";
import { api, Form, Work } from "@/lib/api";
import { dt, overdue } from "@/lib/format";
import { t } from "@/lib/i18n";
import FormRenderer, { validate } from "./FormRenderer";

export default function InlineTask({ work, me, onDone }: { work: Work; me: string; onDone: () => void }) {
  const [form, setForm] = useState<Form>();
  const [d, setD] = useState<Record<string, any>>({});
  const [errs, setErrs] = useState<Record<string, string>>({});
  const [e, setE] = useState<string>();
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    api.get<{ form?: Form; vars?: Record<string, any> }>(`/api/work/${work.id}`).then((v) => {
      setForm(v.form ?? undefined);
      const pre: Record<string, any> = { ...(work.input ?? {}) };
      // per-item tasks (one per reviewer, agency, ...) start blank: case values belong to other items
      if (!("loopCounter" in pre)) for (const f of v.form?.schema.fields ?? []) if (v.vars && f.key in v.vars && !(f.key in pre)) pre[f.key] = v.vars[f.key];
      setD(pre);
    }).catch(() => {});
  }, [work.id, work.input]);
  if (work.ui === "external") {
    return <section className="card task"><h3>{work.name}</h3><Link className="btn-o" href={`/work/${work.id}`}>{t("Open external screen")}</Link></section>;
  }
  const submit = async () => {
    const v = validate(form?.schema.fields ?? [], d);
    setErrs(v);
    if (Object.keys(v).length) return;
    setBusy(true);
    try {
      if (work.assignee !== me) await api.post(`/api/work/${work.id}/claim`);
      const out = { ...d };
      for (const f of form?.schema.fields ?? []) if (f.type === "display" || f.type === "section") delete out[f.key];
      await api.post(`/api/work/${work.id}/complete`, out);
      onDone();
    } catch (x: any) { setE(x.message); } finally { setBusy(false); }
  };
  return (
    <section className="card task">
      <div className="task-head">
        <h3>{work.name} <span className="small muted">· {t("your task")}</span></h3>
        {work.dueAt && <span className={"small" + (overdue(work.dueAt) ? " late-t" : " muted")}>{t("Due")} {dt(work.dueAt)}</span>}
      </div>
      {form ? <FormRenderer fields={form.schema.fields} data={d} onChange={setD} errors={errs} /> : null}
      {e && <div className="err">{e}</div>}
      <div className="bar"><button className="btn" disabled={busy} onClick={submit}>{form ? t("Submit") : t("Mark as done")}</button></div>
    </section>
  );
}

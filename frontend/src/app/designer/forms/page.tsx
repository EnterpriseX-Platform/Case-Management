"use client";
// Forms library (Studio): reusable forms for task screens and actions.
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { api, Form } from "@/lib/api";
import { dt } from "@/lib/format";
import { t } from "@/lib/i18n";
import { useLoad } from "@/lib/useLoad";
import NewItem from "@/components/NewItem";
import { Empty, Err, Page } from "@/components/ui";

export default function Forms() {
  const r = useRouter();
  const { data, error } = useLoad(() => api.get<Form[]>("/api/forms"));
  const [creating, setCreating] = useState(false);
  const [e, setE] = useState<string>();
  const create = (key: string, name: string) =>
    api.put(`/api/forms/${key}`, { key, name, schema: { fields: [] } }).then(() => r.push(`/designer/forms/${key}`)).catch((x) => setE(x.message));
  return (
    <Page title={t("Forms")} actions={!creating && <button className="btn" onClick={() => setCreating(true)}>{t("New form")}</button>}>
      <Err e={error ?? e} />
      {creating && <NewItem label={t("New form")} onCreate={create} onCancel={() => setCreating(false)} />}
      {data?.length ? (
        <table className="tbl">
          <thead><tr><th>{t("Form")}</th><th className="r">{t("Fields")}</th><th className="r">{t("Revision")}</th><th>{t("Updated")}</th></tr></thead>
          <tbody>{data.map((f) => (
            <tr key={f.key}><td><Link href={`/designer/forms/${f.key}`}>{f.name || f.key}</Link><div className="small muted mono dev-only">{f.key}</div></td>
              <td className="r">{f.schema?.fields?.length ?? 0}</td><td className="r">{f.version}</td><td className="nw">{dt((f as any).updatedAt)}</td></tr>
          ))}</tbody>
        </table>
      ) : data && <Empty>-</Empty>}
    </Page>
  );
}

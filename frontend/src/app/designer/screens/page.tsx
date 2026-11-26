"use client";
// Screens library (Studio): layouts for task pages, case pages and role home pages.
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { api, Screen } from "@/lib/api";
import { t } from "@/lib/i18n";
import { useLoad } from "@/lib/useLoad";
import NewItem from "@/components/NewItem";
import { Empty, Err, Page } from "@/components/ui";

const KIND: Record<string, string> = { task: "Task screen", case: "Case overview", portal: "Home page by role", dashboard: "Dashboard" };

export default function Screens() {
  const r = useRouter();
  const { data, error } = useLoad(() => api.get<Screen[]>("/api/screens"));
  const [creating, setCreating] = useState(false);
  const [e, setE] = useState<string>();
  const create = (key: string, name: string) =>
    api.put(`/api/screens/${key}`, { key, name, kind: "task", layout: { rows: [{ cols: [{ width: 8, widget: "form" }, { width: 4, widget: "caseInfo" }] }] } })
      .then(() => r.push(`/designer/screens/${key}`)).catch((x) => setE(x.message));
  return (
    <Page title={t("Screens")} actions={!creating && <button className="btn" onClick={() => setCreating(true)}>{t("New screen")}</button>}>
      <Err e={error ?? e} />
      {creating && <NewItem label={t("New screen")} onCreate={create} onCancel={() => setCreating(false)} />}
      {data?.length ? (
        <table className="tbl">
          <thead><tr><th>{t("Screen")}</th><th>{t("Used for")}</th><th>{t("Roles")}</th></tr></thead>
          <tbody>{data.map((s) => (
            <tr key={s.key}><td><Link href={`/designer/screens/${s.key}`}>{s.name || s.key}</Link><div className="small muted mono dev-only">{s.key}</div></td>
              <td>{t(KIND[s.kind] ?? s.kind)}</td><td>{s.roles?.length ? s.roles.join(", ") : t("All roles")}</td></tr>
          ))}</tbody>
        </table>
      ) : data && <Empty>-</Empty>}
    </Page>
  );
}

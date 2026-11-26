"use client";
import { t } from "@/lib/i18n";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { api, Screen } from "@/lib/api";
import { useLoad } from "@/lib/useLoad";
import { Err, Page } from "@/components/ui";

const KIND: Record<string, string> = { task: "Task screen", case: "Case overview", portal: "Requester portal", dashboard: "Dashboard" };

export default function Screens() {
  const r = useRouter();
  const { data, error } = useLoad(() => api.get<Screen[]>("/api/screens"));
  return (
    <Page title={t("Screens")} actions={<button className="btn" onClick={() => { const k = prompt("Screen key"); if (k) r.push(`/designer/screens/${k}`); }}>{t("New screen")}</button>}>
      <Err e={error} />
      <table className="tbl">
        <thead><tr><th>{t("Key")}</th><th>{t("Name")}</th><th>{t("Used for")}</th><th>{t("Role")}</th></tr></thead>
        <tbody>{(data ?? []).map((s) => (
          <tr key={s.key}><td><Link href={`/designer/screens/${s.key}`}>{s.key}</Link></td><td>{s.name}</td><td>{KIND[s.kind] ?? s.kind}</td><td>{s.roles?.join(", ") ?? "All roles"}</td></tr>
        ))}</tbody>
      </table>
    </Page>
  );
}

"use client";
import Link from "next/link";
import { api, Notification } from "@/lib/api";
import { dt } from "@/lib/format";
import { t } from "@/lib/i18n";
import { useLoad } from "@/lib/useLoad";
import { Empty, Page } from "@/components/ui";

export default function Notifications() {
  const { data, reload } = useLoad(() => api.get<Notification[]>("/api/notifications"));
  return (
    <Page title={t("Notifications")} actions={<button className="btn-o" onClick={() => api.post("/api/notifications/read", {}).then(reload)}>{t("Mark all as read")}</button>}>
      {data?.length ? (
        <table className="tbl"><tbody>{data.map((n) => (
          <tr key={n.id} className={n.read ? "" : "unread"}>
            <td className="nw small">{dt(n.at)}</td><td className="small">{t(n.kind)}</td>
            <td>{n.link ? <Link href={n.link} onClick={() => api.post("/api/notifications/read", { ids: [n.id] })}>{n.title}</Link> : n.title}</td>
          </tr>
        ))}</tbody></table>
      ) : data && <Empty>-</Empty>}
    </Page>
  );
}

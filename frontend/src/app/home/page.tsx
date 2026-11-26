"use client";
// Persona home: the portal screen designed for the user's roles.
import { useRouter } from "next/navigation";
import { api, Screen, Work } from "@/lib/api";
import { overdue } from "@/lib/format";
import { t } from "@/lib/i18n";
import { useLoad } from "@/lib/useLoad";
import { useSession } from "@/lib/useSession";
import ScreenRenderer from "@/components/ScreenRenderer";
import { Page } from "@/components/ui";

const DEFAULT_HOME: Screen = { key: "_home", name: "Home", kind: "portal", layout: { rows: [
  { cols: [{ width: 8, widget: "worklist" }, { width: 4, widget: "newCase" }] },
  { cols: [{ width: 12, widget: "myCases" }] },
] } };

function portalFor(screens: Screen[], roles: string[]) {
  const portals = screens.filter((s) => s.kind === "portal");
  return portals.find((s) => (s.roles ?? []).some((r) => roles.includes(r))) ?? portals.find((s) => !s.roles?.length);
}

export default function Home() {
  const s = useSession();
  const r = useRouter();
  const { data } = useLoad(() => api.get<Screen[]>("/api/screens"), [s?.id]);
  const { data: work } = useLoad(() => api.get<Work[]>("/api/work"), [s?.id]);
  if (!s || !data) return null;
  const screen = portalFor(data ?? [], s.roles) ?? DEFAULT_HOME;
  const w = work ?? [];
  const late = w.filter((x) => overdue(x.dueAt)).length;
  const next = () => api.post<Work>("/api/work/next").then((x) => r.push(`/work/${x.id}`)).catch(() => r.push("/work"));
  return (
    <Page title={`${t("Hello")} ${s.name}`}>
      {w.length > 0 && (
        <div className={"callout" + (late ? " warn-l" : "")}>
          <span>{t("Work waiting for you")}: <b>{w.length}</b>{late ? <> · {t("overdue")} <b className="late-t">{late}</b></> : null}</span>
          <button className="btn-s" onClick={next}>{t("Get next work")}</button>
        </div>
      )}
      <ScreenRenderer screen={screen} ctx={{}} />
    </Page>
  );
}

"use client";
// Persona home: the portal screen designed for the user's roles.
import { api, Screen } from "@/lib/api";
import { t } from "@/lib/i18n";
import { useLoad } from "@/lib/useLoad";
import { useSession } from "@/lib/useSession";
import ScreenRenderer from "@/components/ScreenRenderer";
import { Page } from "@/components/ui";

const DEFAULT_HOME: Screen = { key: "_home", name: "Home", kind: "portal", layout: { rows: [
  { cols: [{ width: 8, widget: "worklist" }, { width: 4, widget: "kpi" }] },
  { cols: [{ width: 12, widget: "newCase" }] },
  { cols: [{ width: 12, widget: "myCases" }] },
] } };

function portalFor(screens: Screen[], roles: string[]) {
  const portals = screens.filter((s) => s.kind === "portal");
  return portals.find((s) => (s.roles ?? []).some((r) => roles.includes(r))) ?? portals.find((s) => !s.roles?.length);
}

export default function Home() {
  const s = useSession();
  const { data } = useLoad(() => api.get<Screen[]>("/api/screens"), [s?.id]);
  if (!s || !data) return null;
  const screen = portalFor(data ?? [], s.roles) ?? DEFAULT_HOME;
  return <Page title={screen.name === "Home" ? t("Home") : screen.name}><ScreenRenderer screen={screen} ctx={{}} /></Page>;
}

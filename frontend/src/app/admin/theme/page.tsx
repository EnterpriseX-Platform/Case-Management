"use client";
// Organisation theme: system name, logo, colours, font, size, corners,
// density and light/dark, with a live preview and a contrast check.
import { t } from "@/lib/i18n";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import Dropdown from "@/components/Dropdown";
import DropZone from "@/components/DropZone";
import { Err, Page } from "@/components/ui";
import { contrast, FONTS, isHex, loadFont, PRESETS, setTheme, Theme, themeVars, useTheme } from "@/lib/theme";

export default function ThemePage() {
  const live = useTheme();
  const [d, setD] = useState<Theme>({});
  const [e, setE] = useState<string>();
  const [msg, setMsg] = useState<string>();
  const [loaded, setLoaded] = useState(false);
  useEffect(() => { if (!loaded && Object.keys(live).length) { setD(live); setLoaded(true); } }, [live, loaded]);
  useEffect(() => { api.get<Theme>("/api/theme").then((x) => { setD(x ?? {}); setLoaded(true); }).catch(() => {}); }, []);
  const set = (p: Partial<Theme>) => { setD({ ...d, ...p }); setMsg(undefined); };
  useEffect(() => loadFont(d.font), [d.font]);
  const accent = isHex(d.accent) ? d.accent! : "#0f62fe";
  const header = isHex(d.header) ? d.header! : "#161616";
  const cAccent = Math.max(contrast(accent, "#ffffff"), contrast(accent, "#161616")), cHeader = Math.max(contrast(header, "#ffffff"), contrast(header, "#161616"));
  const pvDark = d.mode === "dark";
  const logo = (fs: File[]) => {
    const f = fs[0];
    if (!f) return;
    if (f.size > 300_000) { setE(t("The logo is too large (keep it under 300 KB)")); return; }
    const r = new FileReader();
    r.onload = () => set({ logo: String(r.result) });
    r.readAsDataURL(f);
  };
  const save = () => api.put<Theme>("/api/theme", d).then((x) => { setTheme(x); setMsg(t("Theme published")); setE(undefined); }).catch((x) => setE(x.message));

  return (
    <Page title={t("Theme")} actions={<>
      {msg && <span className="small th-ok">{msg}</span>}
      <button className="btn-o" onClick={() => { setD({}); setMsg(undefined); }}>{t("Reset to default")}</button>
      <button className="btn" onClick={save}>{t("Publish theme")}</button>
    </>}>
      <Err e={e} />
      <div className="th">
        <section className="th-form">
          <h3>{t("Ready-made themes")}</h3>
          <div className="th-presets">
            {PRESETS.map((p) => (
              <button key={p.name} className={d.accent === p.theme.accent && d.header === p.theme.header ? "on" : ""} onClick={() => set(p.theme)}>
                <i style={{ background: p.theme.accent }} /><i style={{ background: p.theme.header }} />{t(p.name)}
              </button>
            ))}
          </div>

          <h3>{t("Name and logo")}</h3>
          <label className="fb-prop"><span>{t("System name")}</span><input value={d.product ?? ""} placeholder="Case Management" onChange={(x) => set({ product: x.target.value || undefined })} /></label>
          <label className="fb-prop"><span>{t("Organisation")}</span><input value={d.org ?? ""} onChange={(x) => set({ org: x.target.value || undefined })} /></label>
          <div className="fb-prop"><span>{t("Logo")}</span>
            {d.logo ? <div className="th-logo"><img src={d.logo} alt="" /><button className="btn-s danger" onClick={() => set({ logo: undefined })}>{t("Remove")}</button></div>
              : <DropZone accept="image/png,image/jpeg,image/svg+xml,image/webp" onFiles={logo} label={t("Drop a PNG, SVG or JPEG logo")} />}
          </div>

          <h3>{t("Colours")}</h3>
          <ColorField label={t("Main colour (buttons, links, selection)")} value={d.accent} fallback="#0f62fe" onChange={(v) => set({ accent: v })} />
          <div className={"th-check " + (cAccent >= 4.5 ? "ok" : "bad")}>{t("Text on buttons")}: {cAccent.toFixed(1)} : 1 · {cAccent >= 4.5 ? t("meets WCAG AA") : t("below WCAG AA (4.5 : 1)")}</div>
          <ColorField label={t("Header bar")} value={d.header} fallback="#161616" onChange={(v) => set({ header: v })} />
          <div className={"th-check " + (cHeader >= 4.5 ? "ok" : "bad")}>{t("Text on header")}: {cHeader.toFixed(1)} : 1 · {cHeader >= 4.5 ? t("meets WCAG AA") : t("below WCAG AA (4.5 : 1)")}</div>

          <h3>{t("Type")}</h3>
          <label className="fb-prop"><span>{t("Font")}</span><Dropdown options={FONTS.map((f) => ({ value: f, label: f }))} value={d.font ?? "IBM Plex Sans Thai"} onChange={(v) => set({ font: v === "IBM Plex Sans Thai" ? undefined : v })} /></label>
          <div className="fb-prop"><span>{t("Base text size")}</span>
            <div className="fb-seg th-seg">{[13, 14, 15, 16].map((n) => <button key={n} className={(d.baseSize ?? 14) === n ? "on" : ""} onClick={() => set({ baseSize: n === 14 ? undefined : n })}>{n}</button>)}</div></div>

          <h3>{t("Shape and density")}</h3>
          <div className="fb-prop"><span>{t("Corners")}</span>
            <div className="fb-seg th-seg">{[{ v: 0, l: t("Square") }, { v: 2, l: "2" }, { v: 4, l: "4" }, { v: 8, l: "8" }].map((o) => <button key={o.v} className={(d.radius ?? 0) === o.v ? "on" : ""} onClick={() => set({ radius: o.v || undefined })}>{o.l}</button>)}</div></div>
          <div className="fb-prop"><span>{t("Density")}</span>
            <div className="fb-seg th-seg">{[{ v: "compact", l: t("Compact") }, { v: "normal", l: t("Normal") }, { v: "comfortable", l: t("Comfortable") }].map((o) => <button key={o.v} className={(d.density ?? "normal") === o.v ? "on" : ""} onClick={() => set({ density: o.v === "normal" ? undefined : o.v })}>{o.l}</button>)}</div></div>
          <div className="fb-prop"><span>{t("Appearance")}</span>
            <div className="fb-seg th-seg">{[{ v: "light", l: t("Light") }, { v: "dark", l: t("Dark") }, { v: "system", l: t("Follow device") }].map((o) => <button key={o.v} className={(d.mode ?? "light") === o.v ? "on" : ""} onClick={() => set({ mode: o.v === "light" ? undefined : o.v })}>{o.l}</button>)}</div></div>
          <label className="fb-prop"><span>{t("Footer text")}</span><input value={d.footer ?? ""} onChange={(x) => set({ footer: x.target.value || undefined })} /></label>
        </section>

        <section className="th-side">
          <h3>{t("Preview")}</h3>
          <div className="th-pv" data-theme={pvDark ? "dark" : "light"} style={{ ...(themeVars(d, pvDark) as React.CSSProperties), fontFamily: "var(--font)", fontSize: d.baseSize ? `${d.baseSize}px` : undefined }}>
            <div className="th-pv-hdr">{d.logo && <img src={d.logo} alt="" />}<b>{d.product || "Case Management"}</b><span className="th-sp" /><span>{t("Notifications")}</span><span>{t("Somsri K.")}</span></div>
            <div className="th-pv-body">
              <div className="th-pv-side"><div className="on">{t("My work")}</div><div>{t("Cases")}</div><div>{t("New case")}</div></div>
              <div className="th-pv-main">
                <div className="th-pv-title">{t("Inspect delivery")}</div>
                <div className="th-pv-lbl">{t("Inspection result")}</div>
                <div className="th-pv-choice"><span className="on">{t("Complete")}</span><span>{t("Incomplete")}</span></div>
                <div className="th-pv-lbl">{t("Inspection date")}</div>
                <div className="th-pv-in">26/11/2026</div>
                <table className="th-pv-tbl"><tbody>
                  <tr><td>PR-2026-00128</td><td>{t("Office chairs")}</td><td className="r">142,000</td></tr>
                  <tr><td>PR-2026-00131</td><td>{t("Laser printers")}</td><td className="r">186,600</td></tr>
                </tbody></table>
                <div className="th-pv-btns"><span className="th-pv-btn">{t("Save and continue")}</span><span className="th-pv-link">{t("Case history")}</span></div>
              </div>
            </div>
            {d.footer && <div className="th-pv-foot">{d.footer}</div>}
          </div>
          <div className="small muted mt">{t("Applies to every screen, the portal, notification e-mails and PDFs of this organisation.")}</div>
        </section>
      </div>
    </Page>
  );
}

function ColorField({ label, value, fallback, onChange }: { label: string; value?: string; fallback: string; onChange: (v?: string) => void }) {
  const [txt, setTxt] = useState(value ?? "");
  useEffect(() => setTxt(value ?? ""), [value]);
  return (
    <div className="fb-prop"><span>{label}</span>
      <div className="th-color">
        <input type="color" aria-label={label} value={isHex(value) ? value : fallback} onChange={(x) => onChange(x.target.value)} />
        <input className="mono" value={txt} placeholder={fallback} onChange={(x) => { setTxt(x.target.value); if (isHex(x.target.value)) onChange(x.target.value.toLowerCase()); if (!x.target.value) onChange(undefined); }} />
      </div>
    </div>
  );
}

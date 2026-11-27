"use client";
// Organisation theme: turns a few choices (accent, header, font, size,
// corners, density, light/dark) into the Carbon tokens the app is built on.
import { useEffect, useState } from "react";

export type Theme = {
  product?: string; org?: string; logo?: string; accent?: string; header?: string; font?: string;
  baseSize?: number; radius?: number; density?: string; mode?: string; footer?: string;
};

export const FONTS = ["IBM Plex Sans Thai", "Sarabun", "Noto Sans Thai", "Prompt", "Kanit", "Bai Jamjuree", "Anuphan"];
export const PRESETS: { name: string; theme: Theme }[] = [
  { name: "IBM Carbon", theme: { accent: "#0f62fe", header: "#161616", font: "IBM Plex Sans Thai", radius: 0 } },
  { name: "Government blue", theme: { accent: "#00539c", header: "#00305e", font: "Sarabun", radius: 2 } },
  { name: "Organisation green", theme: { accent: "#1b7f3b", header: "#0e3f1d", font: "Noto Sans Thai", radius: 4 } },
  { name: "Institution purple", theme: { accent: "#6929c4", header: "#31135e", font: "Prompt", radius: 4 } },
  { name: "Local orange", theme: { accent: "#b45309", header: "#3b1d06", font: "Kanit", radius: 8 } },
];

// ---- colour maths (sRGB / WCAG 2.1)
const rgb = (h: string) => [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16));
const hex = (c: number[]) => "#" + c.map((v) => Math.max(0, Math.min(255, Math.round(v))).toString(16).padStart(2, "0")).join("");
const mix = (a: string, b: string, t: number) => { const x = rgb(a), y = rgb(b); return hex(x.map((v, i) => v + (y[i] - v) * t)); };
const lum = (h: string) => { const [r, g, b] = rgb(h).map((v) => { const s = v / 255; return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4; }); return 0.2126 * r + 0.7152 * g + 0.0722 * b; };
export const contrast = (a: string, b: string) => { const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p); return (x + 0.05) / (y + 0.05); };
export const isHex = (s?: string) => !!s && /^#[0-9a-f]{6}$/i.test(s);
const onColor = (bg: string) => (contrast(bg, "#ffffff") >= contrast(bg, "#161616") ? "#ffffff" : "#161616");

/** CSS custom properties for a theme (empty values keep the defaults). */
export function themeVars(t: Theme, dark = false): Record<string, string> {
  const v: Record<string, string> = {};
  if (isHex(t.accent)) {
    const a = dark ? mix(t.accent!, "#ffffff", 0.25) : t.accent!;
    Object.assign(v, {
      "--cds-interactive": a, "--cds-link": a, "--cds-focus": a,
      "--cds-button-primary-hover": mix(a, "#000000", 0.12), "--cds-button-primary-active": mix(a, "#000000", 0.35),
      "--cds-highlight": mix(a, dark ? "#161616" : "#ffffff", 0.82), "--cds-text-on-color": onColor(a),
    });
  }
  if (isHex(t.header)) Object.assign(v, { "--cds-shell-bg": t.header!, "--cm-shell-fg": onColor(t.header!), "--cm-shell-line": mix(t.header!, onColor(t.header!), 0.18), "--cds-shell-text": onColor(t.header!) === "#ffffff" ? "#e0e0e0" : "#161616", "--cds-shell-hover": mix(t.header!, onColor(t.header!), 0.12) });
  if (t.font) v["--font"] = `"${t.font}", "IBM Plex Sans Thai", "IBM Plex Sans", system-ui, sans-serif`;
  if (t.baseSize) v["--cm-base"] = `${t.baseSize}px`;
  v["--cm-radius"] = `${t.radius ?? 0}px`;
  return v;
}

export function loadFont(name?: string) {
  if (!name || name === "IBM Plex Sans Thai") return;
  const id = "cm-font-" + name.replace(/\s/g, "-");
  if (document.getElementById(id)) return;
  const l = document.createElement("link");
  l.id = id; l.rel = "stylesheet";
  l.href = `https://fonts.googleapis.com/css2?family=${encodeURIComponent(name)}:wght@400;600&display=swap`;
  document.head.appendChild(l);
}

const prefersDark = () => typeof window !== "undefined" && window.matchMedia?.("(prefers-color-scheme: dark)").matches;

/** Applies a theme to the whole page. */
export function applyTheme(t: Theme) {
  const root = document.documentElement;
  const dark = t.mode === "dark" || (t.mode === "system" && prefersDark());
  root.dataset.theme = dark ? "dark" : "light";
  root.dataset.density = t.density || "normal";
  for (const k of Array.from(root.style)) if (k.startsWith("--cds-") || k.startsWith("--cm-") || k === "--font") root.style.removeProperty(k);
  for (const [k, val] of Object.entries(themeVars(t, dark))) root.style.setProperty(k, val);
  loadFont(t.font);
  document.title = t.product || "Case Management";
}

const KEY = "cm.theme";
let current: Theme = {};
const subs = new Set<(t: Theme) => void>();

/** Loads the organisation theme once (cached for an instant first paint). */
export function useTheme(): Theme {
  const [t, setT] = useState<Theme>(current);
  useEffect(() => {
    subs.add(setT);
    if (!Object.keys(current).length) {
      try { const c = localStorage.getItem(KEY); if (c) setTheme(JSON.parse(c), false); } catch { /* storage blocked */ }
      fetch("/api/theme").then((r) => (r.ok ? r.json() : null)).then((x) => x && setTheme(x)).catch(() => {});
    }
    return () => { subs.delete(setT); };
  }, []);
  return t;
}

export function setTheme(t: Theme, persist = true) {
  current = t;
  applyTheme(t);
  if (persist) { try { localStorage.setItem(KEY, JSON.stringify(t)); } catch { /* storage blocked */ } }
  subs.forEach((f) => f(t));
}

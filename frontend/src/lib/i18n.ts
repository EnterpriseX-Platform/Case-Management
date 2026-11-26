// UI language. English strings are the keys; other languages map them.
// The choice is kept per browser; switching reloads the page.
import th from "./th.json";

const dicts: Record<string, Record<string, string>> = { th };
export const languages = [{ value: "en", label: "English" }, { value: "th", label: "ไทย" }];

let lang = "en";
if (typeof window !== "undefined") {
  try { lang = localStorage.getItem("cm.lang") || (navigator.language.startsWith("th") ? "th" : "en"); } catch {}
}

export const currentLang = () => lang;

export function t(s: string): string {
  return (lang !== "en" && dicts[lang]?.[s]) || s;
}

export function setLang(l: string) {
  try { localStorage.setItem("cm.lang", l); } catch {}
  location.reload();
}

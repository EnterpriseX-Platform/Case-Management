import { t } from "./i18n";
export const statusLabel: Record<string, string> = {
  draft: "Draft", in_review: "In review", approved: "Approved", published: "Published", deprecated: "Deprecated",
  archived: "Archived", running: "In progress", completed: "Completed", failed: "Failed", cancelled: "Cancelled",
  open: "Unassigned", claimed: "Claimed", done: "Done",
};

export function th(s?: string) {
  return (s && t(statusLabel[s] ?? s)) || "-";
}

export function dt(s?: string) {
  if (!s) return "-";
  const d = new Date(s);
  return d.toLocaleString("en-GB", { dateStyle: "medium", timeStyle: "short" });
}

export function money(v: unknown) {
  const n = Number(v);
  return Number.isFinite(n) ? n.toLocaleString("en-GB", { maximumFractionDigits: 2 }) : String(v ?? "");
}

export function overdue(due?: string) {
  return !!due && new Date(due).getTime() < Date.now();
}

/** Evaluates simple visibleIf expressions like `a < b`, `x == 'y'` against data. */
export function visible(expr: string | undefined, data: Record<string, unknown>) {
  if (!expr) return true;
  try {
    const names = Object.keys(data).filter((k) => /^[A-Za-z_]\w*$/.test(k));
    // eslint-disable-next-line no-new-func
    const fn = new Function(...names, `return (${expr.replace(/([^=!<>])==(?!=)/g, "$1===").replace(/!=(?!=)/g, "!==")});`);
    return !!fn(...names.map((k) => data[k]));
  } catch {
    return true;
  }
}

/** Label of a multi-instance item (e.g. which department a parallel task is for). */
export function loopLabel(input?: Record<string, unknown>) {
  const v = Object.entries(input ?? {}).find(([k]) => k !== "loopCounter");
  return v ? ` · ${String(v[1])}` : "";
}

/** Display value of a case data field: option labels, yes/no, numbers. */
export function fieldValue(v: unknown, f?: { type?: string; options?: { value: string; label: string }[] }) {
  if (v === undefined || v === null || v === "") return "-";
  const opt = (x: unknown) => f?.options?.find((o) => o.value === String(x))?.label ?? String(x);
  if (Array.isArray(v)) return v.map(opt).join(", ");
  if (f?.options?.length) return opt(v);
  if (f?.type === "money" || f?.type === "number" || typeof v === "number") return money(v);
  if (f?.type === "boolean" || typeof v === "boolean") return v ? t("Yes") : t("No");
  return String(v);
}

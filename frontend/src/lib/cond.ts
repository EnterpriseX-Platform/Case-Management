// No-code conditions: evaluated here for the screen, and compiled to an
// expression the server checks with the same meaning.
import type { Cond, CondRule } from "./api";

export const OPS: { value: CondRule["op"]; label: string; needsValue: boolean }[] = [
  { value: "eq", label: "=", needsValue: true }, { value: "ne", label: "≠", needsValue: true },
  { value: "gt", label: ">", needsValue: true }, { value: "gte", label: "≥", needsValue: true },
  { value: "lt", label: "<", needsValue: true }, { value: "lte", label: "≤", needsValue: true },
  { value: "empty", label: "is empty", needsValue: false }, { value: "filled", label: "is filled", needsValue: false },
];

const isNum = (s: string) => s.trim() !== "" && !isNaN(Number(s));
const literal = (s = "") => (s === "true" || s === "false" ? s : isNum(s) ? String(Number(s)) : JSON.stringify(s));
const empty = (v: unknown) => v === undefined || v === null || v === "" || (Array.isArray(v) && !v.length);

function rule(r: CondRule, data: Record<string, unknown>): boolean {
  const v = data[r.field];
  if (r.op === "empty") return empty(v);
  if (r.op === "filled") return !empty(v);
  const want = r.value ?? "";
  if (r.op === "eq" || r.op === "ne") {
    const same = typeof v === "boolean" ? String(v) === want : typeof v === "number" ? isNum(want) && v === Number(want) : String(v ?? "") === want;
    return r.op === "eq" ? same : !same;
  }
  const a = Number(v), b = Number(want);
  if (empty(v) || isNaN(a) || isNaN(b)) return false;
  return r.op === "gt" ? a > b : r.op === "gte" ? a >= b : r.op === "lt" ? a < b : a <= b;
}

export function evalCond(c: Cond | undefined, data: Record<string, unknown>): boolean {
  const rules = (c?.rules ?? []).filter((r) => r.field);
  if (!rules.length) return true;
  return c!.any ? rules.some((r) => rule(r, data)) : rules.every((r) => rule(r, data));
}

/** Compiles to the server's expression language (expr-lang). */
export function compileCond(c: Cond | undefined): string | undefined {
  const rules = (c?.rules ?? []).filter((r) => r.field && /^[A-Za-z_]\w*$/.test(r.field));
  if (!rules.length) return undefined;
  const one = (r: CondRule) => {
    const f = r.field;
    switch (r.op) {
      case "empty": return `(${f} ?? "") == ""`;
      case "filled": return `(${f} ?? "") != ""`;
      case "eq": return `${f} == ${literal(r.value)}`;
      case "ne": return `${f} != ${literal(r.value)}`;
      default: return `(${f} ?? 0) ${{ gt: ">", gte: ">=", lt: "<", lte: "<=" }[r.op]} ${literal(r.value)}`;
    }
  };
  return rules.map(one).join(c!.any ? " || " : " && ");
}

"use client";
// In-page confirm / prompt dialogs (Carbon modal) instead of the browser's
// window.confirm / window.prompt.
import { useState } from "react";
import Modal from "./Modal";
import { t } from "@/lib/i18n";

type Ask = { title: string; label?: string; value?: string; danger?: boolean; ok: string; input: boolean; resolve: (v: string | null) => void };

export function useDialog() {
  const [ask, setAsk] = useState<Ask | null>(null);
  const [val, setVal] = useState("");
  const close = (v: string | null) => { ask?.resolve(v); setAsk(null); };
  const confirm = (title: string, opts: { ok?: string; danger?: boolean } = {}) =>
    new Promise<boolean>((res) => { setVal(""); setAsk({ title, ok: opts.ok ?? t("Confirm"), danger: opts.danger, input: false, resolve: (v) => res(v !== null) }); });
  const prompt = (title: string, label: string, value = "", opts: { ok?: string; danger?: boolean } = {}) =>
    new Promise<string | null>((res) => { setVal(value); setAsk({ title, label, value, ok: opts.ok ?? t("Confirm"), danger: opts.danger, input: true, resolve: res }); });
  const node = ask ? (
    <Modal title={ask.title} onClose={() => close(null)} footer={<>
      <button className="btn-o" style={{ border: 0, background: "var(--cds-button-secondary)", color: "#fff" }} onClick={() => close(null)}>{t("Cancel")}</button>
      <button className={"btn" + (ask.danger ? " danger-fill" : "")} disabled={ask.input && !val.trim()} onClick={() => close(ask.input ? val.trim() : "")}>{ask.ok}</button>
    </>}>
      {ask.input && (
        <label className="fld"><span className="fld-l">{ask.label}</span>
          <input autoFocus value={val} onChange={(e) => setVal(e.target.value)} onKeyDown={(e) => e.key === "Enter" && val.trim() && close(val.trim())} /></label>
      )}
    </Modal>
  ) : null;
  return { confirm, prompt, node };
}

"use client";
// Custom dropdown (no native <select>): searchable, keyboard friendly,
// single or multiple selection.
import { useEffect, useMemo, useRef, useState } from "react";

export type Option = { value: string; label: string; hint?: string };

type Props = {
  options: Option[];
  value: string | string[] | undefined;
  onChange: (v: any) => void;
  multiple?: boolean;
  placeholder?: string;
  disabled?: boolean;
  allowCustom?: boolean;
};

export default function Dropdown({ options, value, onChange, multiple, placeholder = "เลือก", disabled, allowCustom }: Props) {
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  const [hi, setHi] = useState(0);
  const ref = useRef<HTMLDivElement>(null);
  const sel = useMemo(() => (Array.isArray(value) ? value : value ? [value] : []), [value]);

  useEffect(() => {
    const h = (e: MouseEvent) => ref.current && !ref.current.contains(e.target as Node) && setOpen(false);
    document.addEventListener("mousedown", h);
    return () => document.removeEventListener("mousedown", h);
  }, []);

  const list = options.filter((o) => !q || (o.label + " " + o.value).toLowerCase().includes(q.toLowerCase()));
  const label = (v: string) => options.find((o) => o.value === v)?.label ?? v;

  const pick = (v: string) => {
    if (multiple) onChange(sel.includes(v) ? sel.filter((x) => x !== v) : [...sel, v]);
    else {
      onChange(v);
      setOpen(false);
    }
    setQ("");
  };

  return (
    <div className={"dd" + (disabled ? " dd-disabled" : "")} ref={ref}>
      <button type="button" className="dd-btn" disabled={disabled} onClick={() => setOpen(!open)}
        aria-haspopup="listbox" aria-expanded={open}>
        <span className={sel.length ? "" : "muted"}>
          {sel.length ? sel.map(label).join(", ") : placeholder}
        </span>
        <span className="dd-caret">▾</span>
      </button>
      {open && (
        <div className="dd-pop" role="listbox">
          <input autoFocus className="dd-search" placeholder="ค้นหา" value={q}
            onChange={(e) => { setQ(e.target.value); setHi(0); }}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown") setHi(Math.min(hi + 1, list.length - 1));
              else if (e.key === "ArrowUp") setHi(Math.max(hi - 1, 0));
              else if (e.key === "Enter") {
                e.preventDefault();
                if (list[hi]) pick(list[hi].value);
                else if (allowCustom && q) pick(q);
              } else if (e.key === "Escape") setOpen(false);
            }} />
          <div className="dd-list">
            {!multiple && sel.length > 0 && (
              <div className="dd-opt muted" onMouseDown={() => { onChange(""); setOpen(false); }}>— ไม่เลือก —</div>
            )}
            {list.map((o, i) => (
              <div key={o.value} className={"dd-opt" + (i === hi ? " hi" : "") + (sel.includes(o.value) ? " on" : "")}
                onMouseEnter={() => setHi(i)} onMouseDown={(e) => { e.preventDefault(); pick(o.value); }}>
                {multiple && <span className="dd-check">{sel.includes(o.value) ? "✓" : ""}</span>}
                <span>{o.label}</span>
                {o.hint && <span className="muted dd-hint">{o.hint}</span>}
              </div>
            ))}
            {!list.length && allowCustom && q && (
              <div className="dd-opt" onMouseDown={() => pick(q)}>ใช้ “{q}”</div>
            )}
            {!list.length && !(allowCustom && q) && <div className="dd-empty muted">ไม่พบรายการ</div>}
          </div>
        </div>
      )}
    </div>
  );
}

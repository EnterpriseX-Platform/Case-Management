"use client";
import { t } from "@/lib/i18n";
// Drag-and-drop file box (no native "Choose File" button).
import { useRef, useState } from "react";

export default function DropZone({ onFiles, accept, multiple, label }: {
  onFiles: (f: File[]) => void; accept?: string; multiple?: boolean; label?: string;
}) {
  const inp = useRef<HTMLInputElement>(null);
  const [over, setOver] = useState(false);
  const [names, setNames] = useState<string[]>([]);
  const take = (fl: FileList | null) => {
    if (!fl || !fl.length) return;
    const fs = Array.from(fl).slice(0, multiple ? undefined : 1);
    setNames(fs.map((f) => f.name));
    onFiles(fs);
  };
  return (
    <div className={"drop" + (over ? " over" : "")} role="button" tabIndex={0}
      onClick={() => inp.current?.click()}
      onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && inp.current?.click()}
      onDragOver={(e) => { e.preventDefault(); setOver(true); }}
      onDragLeave={() => setOver(false)}
      onDrop={(e) => { e.preventDefault(); setOver(false); take(e.dataTransfer.files); }}>
      <input ref={inp} type="file" hidden accept={accept} multiple={multiple} onChange={(e) => take(e.target.files)} />
      <div>{names.length ? names.join(", ") : label ?? t("Drag and drop a file here or click to upload")}</div>
      {accept && <div className="muted small">{accept}</div>}
    </div>
  );
}

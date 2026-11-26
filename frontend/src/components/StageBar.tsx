"use client";
// Case lifecycle: primary stages as a Carbon progress indicator; alternate
// and resolution stages appear once reached.
import { StageView } from "@/lib/api";
import { dt, overdue } from "@/lib/format";
import { t } from "@/lib/i18n";

export default function StageBar({ stages }: { stages: StageView[] }) {
  if (!stages?.length) return null;
  return (
    <ol className="stages">
      {stages.map((s) => (
        <li key={s.key} className={`stage stage-${s.state} stage-k-${s.kind}`}>
          <span className="stage-dot" aria-hidden>{s.state === "done" ? "✓" : ""}</span>
          <span className="stage-name">{s.name}</span>
          <span className="stage-meta small">
            {s.state === "current" && s.dueAt ? <span className={overdue(s.dueAt) ? "late-t" : ""}>{t("Due")} {dt(s.dueAt)}</span>
              : s.entered ? dt(s.entered) : s.state === "skipped" ? t("Skipped") : ""}
          </span>
        </li>
      ))}
    </ol>
  );
}

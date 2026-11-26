"use client";
// Small charts for the operations views. Colors: categorical series 1/2 and
// the reserved status colors; text always uses text tokens. Every chart has a
// legend and a hover tooltip; the numbers are also shown in tables nearby.
import { useState } from "react";

export type Series = { key: string; label: string; color: string };

export function GroupedBars({ data, series, height = 180, xLabel }: {
  data: { x: string; values: Record<string, number> }[]; series: Series[]; height?: number; xLabel: (x: string) => string;
}) {
  const [hover, setHover] = useState<number | null>(null);
  const W = 640, H = height, padL = 32, padB = 22, padT = 8;
  const max = Math.max(1, ...data.flatMap((d) => series.map((s) => d.values[s.key] ?? 0)));
  const step = niceStep(max);
  const top = Math.ceil(max / step) * step;
  const ticks = Array.from({ length: Math.floor(top / step) + 1 }, (_, i) => i * step);
  const gw = (W - padL) / Math.max(1, data.length);
  const bw = Math.max(2, Math.min(14, (gw - 4) / series.length - 2));
  const y = (v: number) => padT + (H - padT - padB) * (1 - v / top);
  const every = Math.ceil(data.length / 8);
  return (
    <div className="chart">
      <div className="legend">{series.map((s) => <span key={s.key}><i style={{ background: s.color }} />{s.label}</span>)}</div>
      <div style={{ position: "relative" }}>
        <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={series.map((s) => s.label).join(", ")} onMouseLeave={() => setHover(null)}>
          {ticks.map((t) => (
            <g key={t}>
              <line x1={padL} x2={W} y1={y(t)} y2={y(t)} className="grid" />
              <text x={padL - 6} y={y(t) + 4} textAnchor="end" className="axis">{t}</text>
            </g>
          ))}
          {data.map((d, i) => {
            const x0 = padL + i * gw + (gw - series.length * (bw + 2)) / 2;
            return (
              <g key={d.x} onMouseEnter={() => setHover(i)}>
                <rect x={padL + i * gw} y={padT} width={gw} height={H - padT - padB} fill="transparent" />
                {series.map((s, j) => {
                  const v = d.values[s.key] ?? 0;
                  const h = Math.max(0, y(0) - y(v));
                  return v > 0 ? <path key={s.key} d={bar(x0 + j * (bw + 2), y(0), bw, h)} fill={s.color} opacity={hover === null || hover === i ? 1 : 0.45} /> : null;
                })}
                {i % every === 0 && <text x={padL + i * gw + gw / 2} y={H - 6} textAnchor="middle" className="axis">{xLabel(d.x)}</text>}
              </g>
            );
          })}
          <line x1={padL} x2={W} y1={y(0)} y2={y(0)} className="base" />
        </svg>
        {hover !== null && data[hover] && (
          <div className="tip" style={{ left: `${((padL + hover * gw + gw / 2) / W) * 100}%` }}>
            <b>{xLabel(data[hover].x)}</b>
            {series.map((s) => <div key={s.key}><i style={{ background: s.color }} />{s.label}<span>{data[hover].values[s.key] ?? 0}</span></div>)}
          </div>
        )}
      </div>
    </div>
  );
}

/** Horizontal stacked bar for parts of a whole (e.g. SLA status). */
export function StackBar({ parts }: { parts: { key: string; label: string; value: number; color: string }[] }) {
  const total = parts.reduce((a, p) => a + p.value, 0);
  const [hover, setHover] = useState<string | null>(null);
  return (
    <div className="chart">
      <div className="legend">{parts.map((p) => <span key={p.key}><i style={{ background: p.color }} />{p.label} <b>{p.value}</b></span>)}</div>
      <div className="stack" role="img" aria-label={parts.map((p) => `${p.label} ${p.value}`).join(", ")}>
        {total === 0 ? <div className="stack-empty" /> : parts.filter((p) => p.value > 0).map((p) => (
          <div key={p.key} style={{ flex: p.value, background: p.color, opacity: hover && hover !== p.key ? 0.45 : 1 }}
            onMouseEnter={() => setHover(p.key)} onMouseLeave={() => setHover(null)} title={`${p.label}: ${p.value} (${Math.round((p.value / total) * 100)}%)`} />
        ))}
      </div>
    </div>
  );
}

function niceStep(max: number) {
  const raw = max / 4;
  const p = Math.pow(10, Math.floor(Math.log10(raw)));
  const n = raw / p;
  return Math.max(1, (n <= 1 ? 1 : n <= 2 ? 2 : n <= 5 ? 5 : 10) * p);
}

// bar with 2px rounded data end, flat on the baseline
function bar(x: number, base: number, w: number, h: number) {
  const r = Math.min(2, w / 2, h);
  const t = base - h;
  return `M${x},${base}V${t + r}Q${x},${t} ${x + r},${t}H${x + w - r}Q${x + w},${t} ${x + w},${t + r}V${base}Z`;
}

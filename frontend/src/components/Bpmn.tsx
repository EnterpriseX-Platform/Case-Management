"use client";
// bpmn-js wrappers: a full modeler for designers and a read-only viewer that
// highlights where a case currently is and the path it has walked.
import { forwardRef, useEffect, useImperativeHandle, useRef } from "react";
import "bpmn-js/dist/assets/diagram-js.css";
import "bpmn-js/dist/assets/bpmn-js.css";
import "bpmn-js/dist/assets/bpmn-font/css/bpmn-embedded.css";

export type ModelerHandle = { getXML: () => Promise<string>; selected: () => string | undefined };

export const EMPTY = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:bpmndi="http://www.omg.org/spec/BPMN/20100524/DI"
 xmlns:dc="http://www.omg.org/spec/DD/20100524/DC" xmlns:cm="http://enterprisex.io/schema/cm/1.0" id="defs" targetNamespace="http://enterprisex.io/cm">
 <process id="process_1" isExecutable="true"><startEvent id="start"/></process>
 <bpmndi:BPMNDiagram id="d"><bpmndi:BPMNPlane id="p" bpmnElement="process_1">
  <bpmndi:BPMNShape id="start_di" bpmnElement="start"><dc:Bounds x="100" y="100" width="36" height="36"/></bpmndi:BPMNShape>
 </bpmndi:BPMNPlane></bpmndi:BPMNDiagram></definitions>`;

// cm: extension attributes are kept as generic attributes by bpmn-moddle, so
// no custom moddle descriptor is needed for round-trip.

export const Modeler = forwardRef<ModelerHandle, { xml: string; readOnly?: boolean; onSelect?: (id?: string, name?: string, type?: string) => void }>(
  function Modeler({ xml, readOnly, onSelect }, ref) {
    const el = useRef<HTMLDivElement>(null);
    const inst = useRef<any>(null);
    const sel = useRef<string | undefined>(undefined);
    useImperativeHandle(ref, () => ({
      getXML: async () => (await inst.current.saveXML({ format: true })).xml as string,
      selected: () => sel.current,
    }));
    useEffect(() => {
      let dead = false;
      (async () => {
        const Mod = readOnly ? (await import("bpmn-js/lib/NavigatedViewer")).default : (await import("bpmn-js/lib/Modeler")).default;
        if (dead || !el.current) return;
        const m = new Mod({ container: el.current });
        inst.current = m;
        await m.importXML(xml || EMPTY);
        (m.get("canvas") as any).zoom("fit-viewport");
        (m.get("eventBus") as any).on("selection.changed", (e: any) => {
          const s = e.newSelection?.[0]?.businessObject;
          sel.current = s?.id;
          onSelect?.(s?.id, s?.name, s?.$type?.replace("bpmn:", ""));
        });
      })();
      return () => { dead = true; inst.current?.destroy(); };
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [xml, readOnly]);
    return <div className="bpmn" ref={el} />;
  });

export function Viewer({ xml, active = [], done = [] }: { xml: string; active?: string[]; done?: string[] }) {
  const el = useRef<HTMLDivElement>(null);
  useEffect(() => {
    let v: any, dead = false;
    (async () => {
      const V = (await import("bpmn-js/lib/NavigatedViewer")).default;
      if (dead || !el.current) return;
      v = new V({ container: el.current });
      await v.importXML(xml);
      const c = v.get("canvas");
      c.zoom("fit-viewport");
      const reg = v.get("elementRegistry");
      done.forEach((id) => reg.get(id) && c.addMarker(id, "m-done"));
      active.forEach((id) => reg.get(id) && c.addMarker(id, "m-active"));
    })();
    return () => { dead = true; v?.destroy(); };
  }, [xml, active.join(), done.join()]);
  return <div className="bpmn bpmn-view" ref={el} />;
}

/** Read-only flow with figures overlaid on steps and a heat shade by load. */
export function MonitorViewer({ xml, labels, heat, selected, onSelect }: {
  xml: string; labels: Record<string, string>; heat: Record<string, 0 | 1 | 2 | 3>; selected?: string; onSelect?: (id: string) => void;
}) {
  const el = useRef<HTMLDivElement>(null);
  useEffect(() => {
    let v: any, dead = false;
    (async () => {
      const V = (await import("bpmn-js/lib/NavigatedViewer")).default;
      if (dead || !el.current) return;
      v = new V({ container: el.current });
      await v.importXML(xml);
      const c = v.get("canvas"), reg = v.get("elementRegistry"), ov = v.get("overlays");
      c.zoom("fit-viewport");
      for (const [id, lv] of Object.entries(heat)) if (lv && reg.get(id)) c.addMarker(id, `m-heat-${lv}`);
      if (selected && reg.get(selected)) c.addMarker(selected, "m-sel");
      for (const [id, text] of Object.entries(labels)) {
        if (!text || !reg.get(id)) continue;
        const div = document.createElement("div");
        div.className = "ov-label" + (heat[id] === 3 ? " hot" : "");
        div.textContent = text;
        ov.add(id, { position: { bottom: -6, left: 0 }, html: div });
      }
      v.get("eventBus").on("element.click", (e: any) => {
        const bo = e.element?.businessObject;
        if (bo && e.element.type !== "bpmn:Process" && !e.element.waypoints) onSelect?.(bo.id);
      });
    })();
    return () => { dead = true; v?.destroy(); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [xml, JSON.stringify(labels), JSON.stringify(heat), selected]);
  return <div className="bpmn bpmn-monitor" ref={el} />;
}

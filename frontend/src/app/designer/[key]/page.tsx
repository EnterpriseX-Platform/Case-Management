"use client";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useState } from "react";
import { api, ProcessDef, Version } from "@/lib/api";
import { dt } from "@/lib/format";
import { useLoad } from "@/lib/useLoad";
import { has, useSession } from "@/lib/useSession";
import { EMPTY } from "@/components/Bpmn";
import Dropdown from "@/components/Dropdown";
import DropZone from "@/components/DropZone";
import { Err, Page, Status } from "@/components/ui";

type Diff = { added: Ch[]; removed: Ch[]; changed: Ch[] };
type Ch = { id: string; name: string; type: string; fields?: string[] };

const actions: Record<string, { a: string; label: string; role: string }[]> = {
  draft: [{ a: "submit", label: "ส่งตรวจ", role: "cm:designer" }],
  in_review: [{ a: "approve", label: "อนุมัติ", role: "cm:approver" }, { a: "reject", label: "ส่งกลับแก้ไข", role: "cm:approver" }],
  approved: [{ a: "publish", label: "เผยแพร่ใช้งาน", role: "cm:approver" }],
  published: [{ a: "deprecate", label: "เลิกใช้กับเรื่องใหม่", role: "cm:approver" }],
  deprecated: [{ a: "archive", label: "เก็บถาวร", role: "cm:admin" }],
};

export default function Versions() {
  const { key } = useParams<{ key: string }>();
  const s = useSession();
  const r = useRouter();
  const { data: p } = useLoad(() => api.get<ProcessDef>(`/api/processes/${key}`), [key]);
  const { data, error, reload } = useLoad(() => api.get<Version[]>(`/api/processes/${key}/versions`), [key]);
  const [e, setE] = useState<string>();
  const [warn, setWarn] = useState<string[]>([]);
  const [a, setA] = useState(""), [b, setB] = useState("");
  const [diff, setDiff] = useState<Diff>();
  const act = (p: Promise<unknown>) => p.then(reload).catch((x) => setE(x.message));
  const opts = (data ?? []).map((v) => ({ value: v.id, label: `v${v.no} · ${v.fiscalYear ? v.fiscalYear + 543 : "-"} · ${v.status}` }));

  const newDraft = async (xml: string, note: string) => {
    try {
      const res = await api.post<{ version: Version; warnings: string[] }>(`/api/processes/${key}/versions`, { bpmn: xml, note });
      setWarn(res.warnings ?? []);
      r.push(`/designer/versions/${res.version.id}`);
    } catch (x: any) { setE(x.message); }
  };

  return (
    <Page title={p?.name ?? key} actions={has(s, "cm:designer") && <button className="btn" onClick={() => newDraft(EMPTY, "")}>สร้างเวอร์ชันใหม่</button>}>
      <Err e={error ?? e} />
      {warn.map((w) => <div key={w} className="warn">{w}</div>)}
      <table className="tbl">
        <thead><tr><th>เวอร์ชัน</th><th>ปีงบประมาณ</th><th>สถานะ</th><th>หมายเหตุ</th><th>ผู้สร้าง</th><th>ผู้อนุมัติ</th><th>เผยแพร่</th><th /></tr></thead>
        <tbody>
          {(data ?? []).map((v) => (
            <tr key={v.id}>
              <td><Link href={`/designer/versions/${v.id}`}>v{v.no}</Link></td>
              <td>{v.fiscalYear ? v.fiscalYear + 543 : "-"}</td>
              <td><Status s={v.status} /></td>
              <td>{v.note}</td><td>{v.createdBy}</td><td>{v.approvedBy ?? "-"}</td><td className="nw">{dt(v.publishedAt)}</td>
              <td className="nw">
                <Link className="link" href={`/designer/versions/${v.id}/mapping`}>หน้าจอของขั้นตอน</Link>
                {(actions[v.status] ?? []).filter((x) => has(s, x.role)).map((x) => (
                  <button key={x.a} className="link" onClick={() => act(api.post(`/api/versions/${v.id}/${x.a}`))}>{x.label}</button>
                ))}
                {has(s, "cm:designer") && (
                  <button className="link" onClick={() => {
                    const y = prompt("ปีงบประมาณของเวอร์ชันใหม่ (พ.ศ.)", v.fiscalYear ? String(v.fiscalYear + 544) : "");
                    if (y) act(api.post<Version>(`/api/versions/${v.id}/clone`, { fiscalYear: Number(y) - 543 }).then((n) => r.push(`/designer/versions/${n.id}`)));
                  }}>คัดลอกเป็นปีถัดไป</button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      {has(s, "cm:designer") && (
        <section className="card mt">
          <h3>นำเข้า BPMN เป็นเวอร์ชันใหม่</h3>
          <DropZone accept=".bpmn,.xml" onFiles={async (fs) => newDraft(await fs[0].text(), "นำเข้าจาก " + fs[0].name)} />
        </section>
      )}

      <section className="card mt">
        <h3>เทียบเวอร์ชัน</h3>
        <div className="row3">
          <Dropdown placeholder="จาก" value={a} onChange={setA} options={opts} />
          <Dropdown placeholder="ไปยัง" value={b} onChange={setB} options={opts} />
          <div><button className="btn-o" disabled={!a || !b} onClick={() => api.get<Diff>(`/api/versions/${a}/diff?to=${b}`).then(setDiff).catch((x) => setE(x.message))}>เทียบ</button></div>
        </div>
        {diff && (
          <table className="tbl mt">
            <thead><tr><th>การเปลี่ยนแปลง</th><th>ขั้นตอน</th><th>ชนิด</th><th>รายละเอียด</th></tr></thead>
            <tbody>
              {diff.added.map((c) => <tr key={"a" + c.id} className="d-add"><td>เพิ่ม</td><td>{c.name || c.id}</td><td>{c.type}</td><td /></tr>)}
              {diff.removed.map((c) => <tr key={"r" + c.id} className="d-del"><td>ลบ</td><td>{c.name || c.id}</td><td>{c.type}</td><td /></tr>)}
              {diff.changed.map((c) => <tr key={"c" + c.id} className="d-chg"><td>แก้ไข</td><td>{c.name || c.id}</td><td>{c.type}</td><td>{c.fields?.join(", ")}</td></tr>)}
            </tbody>
          </table>
        )}
      </section>
    </Page>
  );
}

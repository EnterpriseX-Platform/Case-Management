"use client";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { api, Screen } from "@/lib/api";
import { useLoad } from "@/lib/useLoad";
import { Err, Page } from "@/components/ui";

const KIND: Record<string, string> = { task: "หน้าทำงานของขั้นตอน", case: "หน้าภาพรวมเรื่อง", portal: "หน้าผู้ยื่นเรื่อง", dashboard: "หน้าติดตามงาน" };

export default function Screens() {
  const r = useRouter();
  const { data, error } = useLoad(() => api.get<Screen[]>("/api/screens"));
  return (
    <Page title="หน้าจอ" actions={<button className="btn" onClick={() => { const k = prompt("รหัสหน้าจอ (ภาษาอังกฤษ)"); if (k) r.push(`/designer/screens/${k}`); }}>สร้างหน้าจอ</button>}>
      <Err e={error} />
      <table className="tbl">
        <thead><tr><th>รหัส</th><th>ชื่อ</th><th>ใช้สำหรับ</th><th>บทบาท</th></tr></thead>
        <tbody>{(data ?? []).map((s) => (
          <tr key={s.key}><td><Link href={`/designer/screens/${s.key}`}>{s.key}</Link></td><td>{s.name}</td><td>{KIND[s.kind] ?? s.kind}</td><td>{s.roles?.join(", ") ?? "ทุกบทบาท"}</td></tr>
        ))}</tbody>
      </table>
    </Page>
  );
}

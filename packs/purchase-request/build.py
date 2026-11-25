#!/usr/bin/env python3
"""Builds the Purchase Request sample pack (flows with diagram layout, forms,
screens, step bindings) into purchase-request.cmpack.

    python3 build.py          # writes flows/*.bpmn and ../purchase-request.cmpack
"""
import json, os, zipfile
from xml.sax.saxutils import escape

HERE = os.path.dirname(os.path.abspath(__file__))

def bpmn(pid, name, nodes, flows):
    """nodes: list of dict(id,type,name,x,y,[attrs],[inner]); flows: (id,src,tgt,[cond])"""
    size = {"startEvent": (36, 36), "endEvent": (36, 36), "exclusiveGateway": (50, 50),
            "parallelGateway": (50, 50), "boundaryEvent": (36, 36)}
    out = ['<?xml version="1.0" encoding="UTF-8"?>',
           '<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" '
           'xmlns:bpmndi="http://www.omg.org/spec/BPMN/20100524/DI" '
           'xmlns:dc="http://www.omg.org/spec/DD/20100524/DC" '
           'xmlns:di="http://www.omg.org/spec/DD/20100524/DI" '
           'xmlns:cm="http://enterprisex.io/schema/cm/1.0" '
           f'id="defs_{pid}" targetNamespace="http://enterprisex.io/cm/packs">',
           f'  <process id="{pid}" name="{escape(name)}" isExecutable="true">']
    for n in nodes:
        attrs = "".join(f' {k}="{escape(str(v))}"' for k, v in n.get("attrs", {}).items())
        inner = n.get("inner", "")
        out.append(f'    <{n["type"]} id="{n["id"]}" name="{escape(n.get("name",""))}"{attrs}>{inner}</{n["type"]}>')
    for f in flows:
        fid, s, t = f[:3]
        if len(f) > 3:
            out.append(f'    <sequenceFlow id="{fid}" sourceRef="{s}" targetRef="{t}"><conditionExpression>{escape(f[3])}</conditionExpression></sequenceFlow>')
        else:
            out.append(f'    <sequenceFlow id="{fid}" sourceRef="{s}" targetRef="{t}"/>')
    out.append('  </process>')
    out.append(f'  <bpmndi:BPMNDiagram id="dia_{pid}"><bpmndi:BPMNPlane id="plane_{pid}" bpmnElement="{pid}">')
    pos = {}
    for n in nodes:
        w, h = size.get(n["type"], (120, 80))
        x, y = n["x"], n["y"]
        if n["type"] in size and n["type"] != "boundaryEvent":
            x, y = x + (120 - w) // 2, y + (80 - h) // 2
        pos[n["id"]] = (x, y, w, h)
        gw = ' isMarkerVisible="true"' if n["type"] == "exclusiveGateway" else ""
        out.append(f'    <bpmndi:BPMNShape id="{n["id"]}_di" bpmnElement="{n["id"]}"{gw}><dc:Bounds x="{x}" y="{y}" width="{w}" height="{h}"/></bpmndi:BPMNShape>')
    for f in flows:
        fid, s, t = f[:3]
        sx, sy, sw, sh = pos[s]
        tx, ty, tw, th = pos[t]
        if tx > sx + sw - 1:
            a, b = (sx + sw, sy + sh // 2), (tx, ty + th // 2)
            pts = [a, b] if a[1] == b[1] else [a, ((a[0] + b[0]) // 2, a[1]), ((a[0] + b[0]) // 2, b[1]), b]
        else:  # loop back: go below
            a, b = (sx + sw // 2, sy + sh), (tx + tw // 2, ty + th)
            low = max(sy + sh, ty + th) + 40
            pts = [a, (a[0], low), (b[0], low), b]
        wp = "".join(f'<di:waypoint x="{x}" y="{y}"/>' for x, y in pts)
        out.append(f'    <bpmndi:BPMNEdge id="{fid}_di" bpmnElement="{fid}">{wp}</bpmndi:BPMNEdge>')
    out.append('  </bpmndi:BPMNPlane></bpmndi:BPMNDiagram>')
    out.append('</definitions>')
    return "\n".join(out) + "\n"

def task(i, name, x, y, role, form=None, sla=None, kind="userTask", **extra):
    a = {"cm:role": role}
    if form: a["cm:form"] = form
    if sla: a["cm:sla"] = sla
    a.update({f"cm:{k}": v for k, v in extra.items()})
    return {"id": i, "type": kind, "name": name, "x": x, "y": y, "attrs": a}

Y, D = 100, 170
main_nodes = [
    {"id": "start", "type": "startEvent", "name": "เริ่มคำขอจัดซื้อ", "x": 0, "y": Y},
    task("request", "จัดทำคำขอจัดซื้อ", D, Y, "requester", "purchase-request", "P3D"),
    dict(task("review", "หน่วยงานที่เกี่ยวข้องให้ความเห็น", 2*D, Y, "reviewer", "opinion-form", "P5D"),
         inner='<multiInstanceLoopCharacteristics cm:collection="reviewers" cm:elementVariable="reviewer"/>'),
    {"id": "review_reminder", "type": "boundaryEvent", "name": "ใกล้ครบกำหนดให้ความเห็น", "x": 2*D + 80, "y": Y + 62,
     "attrs": {"attachedToRef": "review", "cancelActivity": "false"},
     "inner": "<timerEventDefinition><timeDuration>P4D</timeDuration></timerEventDefinition>"},
    task("follow_up", "ติดตามความเห็นที่ยังไม่ได้รับ", 2*D + 40, Y + 200, "requester", "note-form", "P1D"),
    {"id": "follow_end", "type": "endEvent", "name": "", "x": 3*D + 40, "y": Y + 200},
    task("approve", "พิจารณาอนุมัติคำขอ", 3*D, Y, "approver", "approval-form", "P3D"),
    task("limit_check", "ตรวจวงเงินตามอำนาจอนุมัติ", 4*D, Y, "", kind="businessRuleTask", rule="approval-limit-check"),
    {"id": "gw_limit", "type": "exclusiveGateway", "name": "อยู่ในวงเงิน?", "x": 5*D, "y": Y, "attrs": {"default": "f_over"}},
    task("revise", "ปรับปรุงคำขอ", 5*D, Y + 200, "requester", "approval-form", "P2D"),
    task("purchase", "ดำเนินการจัดซื้อ", 6*D, Y, "procurement", "purchase-order", "P14D"),
    task("receive", "ตรวจรับพัสดุ", 7*D, Y, "requester", "receive-form", "P7D"),
    {"id": "accounting", "type": "manualTask", "name": "บันทึกบัญชี", "x": 8*D, "y": Y,
     "attrs": {"cm:role": "finance", "cm:form": "accounting-form"}},
    {"id": "end", "type": "endEvent", "name": "ปิดคำขอ", "x": 9*D, "y": Y},
]
main_flows = [
    ("f1", "start", "request"), ("f2", "request", "review"), ("f3", "review", "approve"),
    ("f4", "approve", "limit_check"), ("f5", "limit_check", "gw_limit"),
    ("f_ok", "gw_limit", "purchase", "${withinLimit == true}"), ("f_over", "gw_limit", "revise"),
    ("f6", "revise", "limit_check"), ("f7", "purchase", "receive"), ("f8", "receive", "accounting"),
    ("f9", "accounting", "end"), ("f10", "review_reminder", "follow_up"), ("f11", "follow_up", "follow_end"),
]
sup_nodes = [
    {"id": "start", "type": "startEvent", "name": "มีความจำเป็นเร่งด่วน", "x": 0, "y": Y},
    task("justification", "ระบุเหตุผลความเร่งด่วน", D, Y, "requester", "urgent-request", "P1D"),
    task("limit_check", "ตรวจวงเงินตามอำนาจอนุมัติ", 2*D, Y, "", kind="businessRuleTask", rule="approval-limit-check"),
    {"id": "gw_limit", "type": "exclusiveGateway", "name": "อยู่ในวงเงิน?", "x": 3*D, "y": Y, "attrs": {"default": "s_no"}},
    {"id": "rejected", "type": "endEvent", "name": "ไม่อนุมัติ", "x": 3*D, "y": Y + 200},
    task("approve", "อนุมัติเร่งด่วน", 4*D, Y, "approver", "approval-form", "PT8H"),
    task("purchase", "ดำเนินการจัดซื้อ", 5*D, Y, "procurement", "purchase-order", "P3D"),
    {"id": "end", "type": "endEvent", "name": "ปิดคำขอ", "x": 6*D, "y": Y},
]
sup_flows = [
    ("s1", "start", "justification"), ("s2", "justification", "limit_check"), ("s3", "limit_check", "gw_limit"),
    ("s_yes", "gw_limit", "approve", "${withinLimit == true}"), ("s_no", "gw_limit", "rejected"),
    ("s4", "approve", "purchase"), ("s5", "purchase", "end"),
]

def F(key, label, type_, **kw):
    d = {"key": key, "label": label, "type": type_}
    d.update(kw)
    return d

forms = {
    "purchase-request": ("คำขอจัดซื้อ", [
        F("amount", "วงเงิน (บาท)", "money", required=True, min=0),
        F("reviewers", "หน่วยงานที่ต้องให้ความเห็น", "multiselect", required=True, source="org"),
        F("items", "รายการ", "table", columns=[F("item", "รายการ", "text"), F("qty", "จำนวน", "number"), F("price", "ราคา", "money")]),
        F("attachments", "เอกสารประกอบ", "file")]),
    "opinion-form": ("ความเห็นหน่วยงาน", [
        F("reviewer", "หน่วยงาน", "display"),
        F("opinion", "ความเห็น", "radio", required=True, options=[{"value": "agree", "label": "เห็นด้วย"}, {"value": "disagree", "label": "ไม่เห็นด้วย"}]),
        F("comment", "หมายเหตุ", "textarea")]),
    "approval-form": ("ผลการพิจารณา", [
        F("approvedAmount", "วงเงินที่อนุมัติ (บาท)", "money", required=True, min=0),
        F("reason", "เหตุผลการปรับลด", "textarea", visibleIf="approvedAmount < amount"),
        F("remark", "ความเห็นผู้พิจารณา", "textarea", required=True)]),
    "purchase-order": ("ใบสั่งซื้อ", [
        F("poNumber", "เลขที่ใบสั่งซื้อ", "text", required=True),
        F("vendor", "ผู้ขาย", "text", required=True),
        F("poDate", "วันที่สั่งซื้อ", "date", required=True)]),
    "receive-form": ("ตรวจรับพัสดุ", [
        F("result", "ผลการตรวจรับ", "radio", required=True, options=[{"value": "accepted", "label": "ครบถ้วน"}, {"value": "partial", "label": "ไม่ครบ"}]),
        F("receiveDate", "วันที่ตรวจรับ", "date", required=True)]),
    "accounting-form": ("บันทึกบัญชี", [
        F("voucher", "เลขที่เอกสารบัญชี", "text", required=True),
        F("postDate", "วันที่บันทึก", "date", required=True)]),
    "urgent-request": ("คำขอจัดซื้อเร่งด่วน", [
        F("amount", "วงเงิน (บาท)", "money", required=True, min=0),
        F("necessity", "เหตุผลความเร่งด่วน", "textarea", required=True)]),
    "note-form": ("บันทึกการติดตาม", [F("note", "บันทึก", "textarea", required=True)]),
}

screens = {
    "task-standard": {"name": "หน้าทำงานมาตรฐาน", "kind": "task", "layout": {"rows": [
        {"cols": [{"width": 8, "widget": "form"}, {"width": 4, "widget": "caseInfo"}]},
        {"cols": [{"width": 6, "widget": "attachments"}, {"width": 6, "widget": "history"}]}]}},
    "task-approval": {"name": "หน้าพิจารณาอนุมัติ", "kind": "task", "roles": ["approver"], "layout": {"rows": [
        {"cols": [{"width": 12, "widget": "caseInfo"}]},
        {"cols": [{"width": 7, "widget": "form"}, {"width": 5, "widget": "relatedCases"}]},
        {"cols": [{"width": 12, "widget": "history"}]}]}},
    "case-overview": {"name": "ภาพรวมเรื่อง", "kind": "case", "layout": {"rows": [
        {"cols": [{"width": 12, "widget": "caseInfo"}]},
        {"cols": [{"width": 12, "widget": "flow"}]},
        {"cols": [{"width": 6, "widget": "subCases"}, {"width": 6, "widget": "history"}]}]}},
}

bindings_main = [
    {"nodeId": "approve", "ui": "form", "formKey": "approval-form", "screenKey": "task-approval"},
    {"nodeId": "revise", "ui": "form", "formKey": "approval-form", "screenKey": "task-approval"},
    {"nodeId": "accounting", "ui": "form", "formKey": "accounting-form", "screenKey": "task-standard"},
]

def main():
    os.makedirs(os.path.join(HERE, "flows"), exist_ok=True)
    files = {
        "flows/purchase-request.bpmn": bpmn("purchase_request", "คำขอจัดซื้อ", main_nodes, main_flows),
        "flows/purchase-request-urgent.bpmn": bpmn("purchase_request_urgent", "คำขอจัดซื้อเร่งด่วน", sup_nodes, sup_flows),
        "bindings/purchase-request.json": json.dumps(bindings_main, ensure_ascii=False, indent=2),
    }
    for k, (name, fields) in forms.items():
        files[f"forms/{k}.json"] = json.dumps({"key": k, "name": name, "schema": {"fields": fields}}, ensure_ascii=False, indent=2)
    for k, s in screens.items():
        files[f"screens/{k}.json"] = json.dumps(dict(key=k, **s), ensure_ascii=False, indent=2)
    files["manifest.json"] = json.dumps({
        "name": "Purchase Request Sample", "version": "1.0.0",
        "processes": [
            {"key": "purchase-request", "name": "คำขอจัดซื้อ", "category": "procurement",
             "bpmn": "flows/purchase-request.bpmn", "bindings": "bindings/purchase-request.json"},
            {"key": "purchase-request-urgent", "name": "คำขอจัดซื้อเร่งด่วน", "category": "procurement",
             "parentKey": "purchase-request", "bpmn": "flows/purchase-request-urgent.bpmn"}],
        "forms": ["forms/*.json"], "screens": ["screens/*.json"]}, ensure_ascii=False, indent=2)
    for name, body in files.items():
        p = os.path.join(HERE, name)
        os.makedirs(os.path.dirname(p), exist_ok=True)
        with open(p, "w", encoding="utf-8") as f:
            f.write(body)
    out = os.path.join(HERE, "..", "purchase-request.cmpack")
    with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as z:
        for name in sorted(files):
            z.write(os.path.join(HERE, name), name)
    print("wrote", os.path.normpath(out))

if __name__ == "__main__":
    main()

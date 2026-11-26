# แผนพัฒนา Case Management ให้เทียบเท่า Pega / IBM BAW

> สถานะเอกสาร: ร่างเพื่อเคาะ · 26 พ.ย. 2569
> ขอบเขต: product กลาง (ไม่มีข้อมูลลูกค้า) · Go backend · Next.js frontend · IBM Carbon · ภาษาอังกฤษเป็นหลัก

---

## 1. สรุปสั้น

ตอนนี้ระบบมี **แกน workflow ที่ทำงานได้จริง**: BPMN engine, version รายปี, งานตามบทบาท, ฟอร์มพื้นฐาน, import และหน้าจอครบ 5 role (โค้ด ~7,300 บรรทัด)
แต่ยัง **ไม่ใช่ case management platform** ในความหมายของ Pega / BAW เพราะขาด 3 แกนหลัก:

| แกน | สิ่งที่ขาด |
|---|---|
| **Case model** | ยังเป็น "process ที่มีเลขเรื่อง" ไม่มี stage, case data model, เอกสาร, ความเห็น, ความสัมพันธ์ระหว่างเรื่อง, งานที่ผู้ใช้เลือกทำเอง (optional action) |
| **Low-code studio** | เครื่องมือแยกกันเป็นหน้า ๆ ไม่มีที่ออกแบบ data model, กฎ, decision table, SLA, routing, integration, persona และไม่มีที่รวมทั้ง app |
| **Monitor / operations** | มีแค่ตารางงานค้าง ไม่มี process monitor แบบ live, heatmap, SLA dashboard, การจัดการเรื่องที่ error, รายงาน, การวิเคราะห์ประสิทธิภาพ |

แผนนี้แบ่งงานเป็น **6 เฟส ใช้เวลา ~24 สัปดาห์** (ทีม 3–4 คน หรือเทียบเท่า) แต่ละเฟสส่งมอบของที่ใช้ได้จริง และมีเกณฑ์รับงานชัดเจน

---

## 2. ศึกษาความสามารถอ้างอิง

### 2.1 Pega Platform (Infinity / Constellation)

| หมวด | ความสามารถหลัก |
|---|---|
| Case lifecycle | Case type ประกอบด้วย **Stage → Process → Step** · primary / alternate stage · resolution stage · ข้าม stage ตามเงื่อนไข · optional action (ผู้ใช้กดเพิ่มงานเองระหว่างทาง) · optional process |
| Case hierarchy | parent / child case · spin-off case · wait for child (all / any / specific) · related case · case dependency |
| Data | data object (case data model) · data reference / page list · data page (ดึงข้อมูลภายนอกแบบ cache) · calculated field (declare expression) · validation rule |
| UI | Constellation: view ประกอบจาก template (form / details / table / tabs) · ไม่เขียน HTML · persona-based portal · responsive · localization |
| Rules | When rule · decision table · decision tree · map value · declare expression · validate rule · rule versioning / ruleset |
| Assignment | routing ไปคน / work queue / skill / ภาระงาน (balanced) · get next work · pull/push · delegation · reassign · bulk action |
| SLA | goal / deadline / passed-deadline · urgency · escalation action (แจ้งเตือน, ย้ายคิว, เพิ่ม urgency) · SLA ระดับ case / stage / step |
| Collaboration | pulse (ความเห็น, @mention) · follow case · attachment พร้อม category · email ขาเข้าเปิด/ต่อ case · notification (email, in-app, push) |
| Integration | REST / SOAP connector · data page · service (expose REST) · queue processor · job scheduler |
| Studio | **App Studio** (business): case designer, data designer, UX, persona, channel · **Dev Studio** (technical) · branch / merge · test case / scenario test · deployment pipeline |
| Ops / Insight | **Insights** (report แบบลากวาง, chart, dashboard) · Process Mining · case explorer · broken process queue · Admin Studio (agents, queue, log) · PDC monitor |
| Security | access group · role · privilege · attribute-based access (ABAC) · field-level security · audit trail |

### 2.2 IBM Business Automation Workflow (BAW) + Case Manager

| หมวด | ความสามารถหลัก |
|---|---|
| Process | BPMN 2.0 เต็ม · process app / toolkit (นำกลับมาใช้ซ้ำ) · snapshot = version · instance migration · process inspector (ดู token, แก้ data, retry, ย้าย) |
| Case | case type · **case property** · **case folder** (เอกสาร FileNet) · **discretionary task** (ผู้ใช้สร้างงานเอง) · activity ที่เริ่มด้วยเงื่อนไข (precondition) · stage · case history / timeline |
| UI | **Coaches** (หน้าจอ low-code จาก view ที่นำกลับมาใช้ซ้ำ) · Process Portal · Workplace · dashboard ในตัว |
| Rules | ODM / decision service · BAW decision table |
| Ops | **Process Inspector** · **Performance Data Warehouse** · tracking group · team performance dashboard · SLA / KPI · process federation |
| Integration | REST / SOAP / JDBC / event emitter · external activity · Automation Services |
| Governance | process app versioning · track (branch) · approval ก่อน deploy · environment variable แยกต่อ environment |

### 2.3 ความสามารถที่ทั้งสองมีร่วมกัน (= ขั้นต่ำของ "full feature")

1. Case model มี **stage** และ **data model** ของตัวเอง
2. **Low-code studio เดียว** ครอบทั้ง app (case, data, UI, rule, SLA, routing, integration, persona)
3. **Work management**: work queue, get-next-work, skill / workload routing, delegation, bulk action
4. **SLA + escalation** หลายระดับ
5. **เอกสาร + ความเห็น + แจ้งเตือน** ในเรื่อง
6. **Process inspector**: แก้ instance ที่ค้าง / error ได้
7. **Monitor + report + dashboard** ในตัว
8. **Versioning + migration + การ deploy ข้าม environment**
9. **Security** ระดับ role / attribute / field + audit

---

## 3. ตารางเทียบความสามารถ (gap)

สัญลักษณ์: ✅ มีครบ · 🟡 มีบางส่วน · ❌ ไม่มี

| # | ความสามารถ | Pega | BAW | ของเรา | หมายเหตุสถานะปัจจุบัน |
|---|---|---|---|---|---|
| 1 | BPMN engine (token, gateway, event, sub-process, MI, call) | 🟡 | ✅ | ✅ | ผ่าน test ครบ MVP node |
| 2 | Stage (case lifecycle) | ✅ | ✅ | ❌ | มีแค่ flow |
| 3 | Case data model (typed field, ใช้ร่วมทุกฟอร์ม) | ✅ | ✅ | ❌ | เก็บเป็น vars แบบอิสระ |
| 4 | Calculated field / validation rule | ✅ | ✅ | ❌ | |
| 5 | Parent / child / related case | ✅ | ✅ | 🟡 | มี call activity + parent id ไม่มี UI สร้าง/ผูก |
| 6 | Optional action / discretionary task | ✅ | ✅ | ❌ | |
| 7 | Form designer ลากวาง + layout + section | ✅ | ✅ | 🟡 | เพิ่มช่องได้ ไม่มีลากวาง/section/tab |
| 8 | View / screen ที่ใช้ซ้ำ (template) | ✅ | ✅ | 🟡 | มี screen layout 12 คอลัมน์ |
| 9 | Persona portal (หน้าแรกตาม role) | ✅ | ✅ | 🟡 | เมนูตาม role แต่หน้าแรกตายตัว |
| 10 | Decision table / when rule | ✅ | ✅ | ❌ | มีแค่ expression ใน gateway และเรียก rule engine ภายนอก |
| 11 | Routing: queue / skill / workload / get-next | ✅ | ✅ | 🟡 | queue ตาม role + claim เท่านั้น |
| 12 | Delegation / out-of-office | ✅ | ✅ | ❌ | |
| 13 | Bulk action | ✅ | ✅ | ❌ | |
| 14 | SLA goal / deadline / escalation | ✅ | ✅ | 🟡 | มี SLA → due date ไม่มี goal / escalation |
| 15 | Business calendar (วันหยุด / เวลาทำการ) | ✅ | ✅ | ❌ | SLA นับเวลาปฏิทิน |
| 16 | เอกสารแนบ + หมวดเอกสาร + preview | ✅ | ✅ | ❌ | ฟอร์มเก็บแค่ชื่อไฟล์ |
| 17 | ความเห็น / @mention / follow | ✅ | 🟡 | ❌ | |
| 18 | แจ้งเตือน (email / in-app) | ✅ | ✅ | ❌ | |
| 19 | ค้นหาเรื่องแบบเต็ม + filter ที่บันทึกไว้ | ✅ | ✅ | 🟡 | filter 3 ตัว |
| 20 | Integration designer (REST connector, data page) | ✅ | ✅ | 🟡 | service task ยิง URL ได้ ไม่มีหน้าออกแบบ/ mapping |
| 21 | Expose API / webhook ขาออก | ✅ | ✅ | 🟡 | REST มี · webhook ไม่มี |
| 22 | App Studio รวมทุกอย่าง | ✅ | ✅ | ❌ | เครื่องมือแยกกัน |
| 23 | ทดสอบ flow ใน studio (run / step-through) | ✅ | ✅ | ❌ | |
| 24 | Version + approval + migration | ✅ | ✅ | ✅ | มี diff / dry-run / mapping |
| 25 | Promote ข้าม environment (export/import app) | ✅ | ✅ | 🟡 | มี pack import ไม่มี export app |
| 26 | Process monitor แบบ live บนแผนภาพ | ✅ | ✅ | ❌ | |
| 27 | Process inspector (แก้ instance / retry / แก้ data) | ✅ | ✅ | 🟡 | ย้ายขั้นตอน + ยกเลิกได้ |
| 28 | Error / incident queue | ✅ | ✅ | ❌ | instance failed เงียบ |
| 29 | SLA dashboard / workload / throughput / cycle time | ✅ | ✅ | ❌ | มีตารางงานค้าง |
| 30 | Report builder + export | ✅ | ✅ | ❌ | |
| 31 | Process mining (variant, คอขวด) | ✅ | 🟡 | ❌ | |
| 32 | RBAC + ABAC + field-level security | ✅ | ✅ | 🟡 | role เท่านั้น |
| 33 | Audit trail ครบ | ✅ | ✅ | ✅ | |
| 34 | Multi-tenant | ✅ | 🟡 | 🟡 | schema รองรับ ยังไม่แยกจริง |
| 35 | Localization (หลายภาษา) | ✅ | ✅ | ❌ | อังกฤษอย่างเดียว |
| 36 | Import จากระบบอื่น (BPMN / Pega / Excel) | — | 🟡 | 🟡 | BPMN, pack, CSV, XLSX |
| 37 | ติดตั้งที่ลูกค้า (container, HA, backup) | ✅ | ✅ | ❌ | |

**สรุป:** ครบ 3 / 37 · บางส่วน 15 / 37 · ไม่มี 19 / 37

---

## 4. สถาปัตยกรรมเป้าหมาย

```
                 ┌──────────────── Next.js (Carbon) ────────────────┐
                 │ App Studio │ Workspace (persona) │ Monitor │ Admin │
                 └──────────────────────┬───────────────────────────┘
                                        │ REST + SSE (live)
┌───────────────────────────── Go API ──┴────────────────────────────────┐
│ Studio svc   │ Case svc     │ Work svc      │ Monitor svc │ Admin svc   │
│ (model,ver,  │ (stage,data, │ (queue,route, │ (metrics,   │ (tenant,IAM,│
│  test-run)   │  docs,notes) │  SLA,deleg.)  │  inspector) │  env,export)│
├──────────────┴──────────────┴───────────────┴─────────────┴─────────────┤
│ Case engine = Stage layer  ▸  BPMN token engine (มีอยู่)  ▸  Rule engine │
│ Event bus (outbox in Postgres → LISTEN/NOTIFY / NATS)                   │
│ Workers: timer · SLA · connector retry · notification · metrics rollup  │
├──────────────────────────────────────────────────────────────────────────┤
│ PostgreSQL (case, work, event, metric) · Object store (S3/MinIO) │
│ Full-text search (Postgres FTS → OpenSearch ภายหลัง)                     │
└──────────────────────────────────────────────────────────────────────────┘
```

หลักการ:
- **Case engine ครอบ BPMN engine**: stage คือชั้นบน แต่ละ stage มี process (BPMN) ได้ ≥ 1 ตัว ใช้ engine เดิม ไม่เขียนใหม่
- **ทุกอย่างที่ออกแบบใน studio คือ "artifact" ที่มี version**: case type, data type, view, rule, SLA, routing, connector รวมกันเป็น **Application version** เดียว ใช้วงจร draft → review → publish เดิม
- **Event outbox**: ทุกการเปลี่ยนแปลงของ case และงานเขียน event ลงตาราง ใช้ทำ monitor แบบ live, report, notification, webhook และ process mining จากแหล่งเดียว
- **Monitor ไม่ query ตารางงานสด**: worker สรุปเป็น metric รายนาที/ชั่วโมง/วันลงตาราง metric

---

## 4.1 Workflow engine ของเราเอง (ตัดสินใจแล้ว: ไม่พึ่ง Temporal / Camunda / Flowable)

### ทำไมทำเอง
- **ไม่ผูกกับผู้ขาย** ทั้ง license (Camunda 8 เปลี่ยนเป็นเชิงพาณิชย์), infra (Temporal ต้องมี cluster + Cassandra/Postgres + worker) และรูปแบบการเขียน (Temporal เขียน flow เป็นโค้ด ซึ่งขัดกับ low-code)
- **case management ต้องการมากกว่า BPMN**: stage, optional action, discretionary task, sentry แบบ CMMN, SLA หลายระดับ, การแก้ instance โดยมนุษย์ ไม่มี engine สำเร็จรูปตัวไหนทำครบ ต้องเขียนชั้นบนเองอยู่แล้ว
- **ติดตั้งที่ลูกค้าราชการง่าย**: binary Go ตัวเดียว + Postgres ไม่มี broker หรือ cluster เพิ่ม
- แกนที่มีอยู่ทำงานได้แล้ว (token engine, MVP node ผ่าน test ครบ) ต่อยอดถูกกว่าย้าย

**กันไม่ให้ผูกกับตัวเราเองด้วย:** นิยาม flow เก็บเป็น **BPMN 2.0 มาตรฐาน** (+ attribute `cm:`) นำเข้า/ส่งออกกับเครื่องมืออื่นได้เสมอ

### หลักการออกแบบ
1. **Engine เป็น pure function**: `(definition, instance, trigger) → (instance', effects)` ไม่แตะ DB หรือเครือข่ายเอง ทำให้ test, จำลอง, replay และย้ายที่เก็บข้อมูลได้ (โครงนี้มีอยู่แล้ว)
2. **ทุกงานที่ออกนอกระบบเป็น job แบบ async**: service task, connector, ส่งอีเมล, webhook ไม่ยิงใน transaction ของ case อีกต่อไป
3. **Exactly-once ต่อ step ภายในระบบ, at-least-once + idempotency key กับภายนอก**
4. **Append-only event log** ต่อ instance คือความจริงหลัก snapshot เป็นแค่ cache
5. **ไม่มี dependency ที่เปลี่ยนแทนไม่ได้**: ใช้ Go stdlib + pgx + ตัวประเมิน expression ที่อยู่หลัง interface ของเราเอง

### ชั้นของ engine
```
┌ Case layer ─────────────────────────────────────────────┐
│ stage · sentry (เงื่อนไขเข้า/ออก) · optional action ·     │
│ discretionary task · child case · case-level SLA         │
├ Process runtime (BPMN token engine) ────────────────────┤
│ token · scope · gateway · event · MI · compensation      │
├ Definition compiler ────────────────────────────────────┤
│ BPMN XML → graph ที่ validate แล้ว + cache ตาม checksum  │
├ Job executor ───────────────────────────────────────────┤
│ connector · script ยาว · notification · retry/backoff ·  │
│ incident · lease + heartbeat · SKIP LOCKED               │
├ Scheduler ──────────────────────────────────────────────┤
│ timer · SLA · business calendar · cycle                  │
├ Persistence ────────────────────────────────────────────┤
│ instance snapshot (+revision) · event log · job · timer  │
│ · outbox (ทั้งหมดใน Postgres, ใน transaction เดียว)       │
└─────────────────────────────────────────────────────────┘
```

### จุดอ่อนของ engine ปัจจุบันที่ต้องแก้ (ตรวจจากโค้ดจริง)
| # | ปัญหา | ผลกระทบ | แก้อย่างไร |
|---|---|---|---|
| E1 | บันทึก case ทับทั้งก้อน ไม่มีตรวจ revision | สองคนกดส่งงานในเรื่องเดียวกันพร้อมกัน ข้อมูลหาย | `revision` + `UPDATE … WHERE revision = $n` · ชนกันให้ retry อัตโนมัติ |
| E2 | service task ยิง HTTP ใน transaction | transaction ค้างนาน, ยิงซ้ำเมื่อ rollback, ระบบภายนอกช้าทำให้ทั้งระบบช้า | เปลี่ยนเป็น job: token รอสถานะ `job` → worker ทำ → ส่งผลกลับเป็น trigger |
| E3 | error ที่ไม่ใช่ BPMN error ทำให้ instance `failed` ทั้งเรื่อง | เรื่องตายถาวร | สร้าง **incident** ที่ token นั้น เรื่องยังเดินส่วนอื่นได้ · retry / แก้ data / ข้ามได้ |
| E4 | ประวัติเก็บใน instance (โตไม่หยุด) | เรื่องยาว ๆ ช้า | ย้ายไปตาราง event log แยก |
| E5 | message ส่งข้ามเรื่องด้วยการ scan ทุก case | ช้าเมื่อเรื่องเยอะ | ตาราง subscription (ชื่อ message + correlation key → token) |
| E6 | ยังไม่มี compensation, transaction, complex gateway, event sub-process | BPMN ไม่ครบ | ทำในเฟส 1–2 ตามลำดับความจำเป็น |
| E7 | ไม่มี benchmark | ไม่รู้ขีดจำกัด | ตั้งเกณฑ์และวัดทุก release |

### เกณฑ์คุณภาพของ engine
- **Conformance**: ผ่านชุดตัวอย่าง BPMN MIWG สำหรับ import/export และ scenario test ราย node ทุกตัวที่ประกาศว่ารองรับ
- **ความทนทาน**: ฆ่า process กลาง job / กลาง transaction แล้วเรื่องต้องเดินต่อถูกต้อง ไม่ซ้ำ ไม่หาย (chaos test ใน CI)
- **ความถูกต้องเมื่อทำพร้อมกัน**: test 100 goroutine กดส่งงานเรื่องเดียวกัน → สำเร็จ 1 ที่เหลือได้ conflict
- **ประสิทธิภาพ (1 node, Postgres ปกติ)**: ≥ 200 step/วินาที · timer คลาดไม่เกิน 10 วินาที · 1 ล้านเรื่องในฐานยังเปิดงานได้ < 300 ms
- **Replay**: สร้าง instance ใหม่จาก event log ได้ตรงกับ snapshot ทุกครั้ง (property test)

### อะไรที่ไม่ทำเอง
- ตัวแปลง expression: ใช้ library ที่มี (expr-lang) ซ่อนหลัง interface `Evaluator` เปลี่ยนเป็น FEEL ภายหลังได้
- ฐานข้อมูล: Postgres อย่างเดียว ไม่ต้องมี Kafka/NATS (เพิ่มได้ภายหลังผ่าน outbox)
- ตัววาด BPMN: bpmn-js (open source, MIT-like license ของ bpmn.io) ฝั่งหน้าจอเท่านั้น

---

## 4.2 UX/UI: ใช้ได้ทั้ง business user และ developer

### แนวคิด: studio เดียว สองโหมด บนโมเดลเดียวกัน
ทั้งสองโหมดแก้ **artifact ชุดเดียวกัน** (case type, data, view, rule, connector) ไม่ใช่สองระบบ business ร่างได้ developer ต่อยอดได้ โดยไม่ต้องแปลงไปมา (แนวเดียวกับ Pega App Studio / Dev Studio)

| | **Business mode** (ค่าเริ่มต้น) | **Developer mode** |
|---|---|---|
| ผู้ใช้ | เจ้าของกระบวนงาน, BA | developer, architect |
| Case / flow | แถบ stage + การ์ด step แบบลากวาง · เพิ่ม step จากเมนูสำเร็จรูป (อนุมัติ, กรอกฟอร์ม, ส่งเรื่องต่อ, รอเอกสาร, เรียกระบบ) | BPMN เต็มทุก element · แก้ XML ได้ · ดู token path |
| Data | ตารางช่องข้อมูล ภาษาคน (ชื่อ, ชนิด, บังคับ) | ชนิดซับซ้อน, reference, calculated expression, index ค้นหา |
| UI | ลากวางช่องลง template (ฟอร์ม, รายละเอียด, แท็บ) · preview ตาม persona | กำหนด component เอง, ฉีด custom widget (React) ผ่าน SDK |
| Rule | decision table แบบ spreadsheet · เงื่อนไขแบบเลือกจากเมนู ("วงเงิน มากกว่า 1,000,000") | expression editor + autocomplete + test case |
| Integration | เลือก connector สำเร็จรูป → map ช่องข้อมูลด้วยการลากเส้น | REST/GraphQL เอง, script ก่อน/หลังเรียก, secret, retry policy |
| คุณภาพ | ตรวจความครบถ้วนแบบ checklist ("step นี้ยังไม่มีผู้รับผิดชอบ") | lint, unit/scenario test, diff, branch, CI |
| เผยแพร่ | ปุ่ม "ส่งขออนุมัติ" | export/import app, pipeline ข้าม environment, API |

### หลัก UX สำหรับ business user
1. **ไม่ต้องรู้ BPMN**: เห็นแค่ stage และ step เป็นภาษางาน ระบบแปลงเป็น BPMN ให้เอง
2. **ตั้งค่าจากเมนู ไม่ต้องพิมพ์ expression**: เงื่อนไข, ผู้รับผิดชอบ, SLA เลือกจากรายการที่อ้าง data model จริง
3. **เห็นผลทันที**: preview หน้าจอตาม persona และปุ่ม **Run** จำลองเรื่องได้จากใน studio
4. **กันพลาดก่อนเผยแพร่**: ตรวจความครบถ้วนอัตโนมัติ เผยแพร่ไม่ได้ถ้ายังมีจุดแดง
5. **แก้กลับได้เสมอ**: undo/redo, ประวัติการแก้ราย artifact, draft แยกจากของที่ใช้งาน
6. **เริ่มจาก template**: คำขออนุมัติ, เรื่องร้องเรียน, จัดซื้อ, onboarding ฯลฯ
7. **Carbon + กติกาเดิม**: dropdown/กล่องลากวางของระบบ, ไม่มีตัวโหลดหมุน, ไม่มีข้อความสอนวิธีใช้บนจอ

### เครื่องมือสำหรับ developer
- **CLI `cm`**: pull/push app เป็นไฟล์ (YAML + BPMN) → เก็บใน git, review ได้
- **SDK**: Go (custom connector, job handler) · TypeScript (custom widget, embed หน้าจองานในระบบอื่น)
- **API ครบ + OpenAPI spec** · webhook ขาออก · event stream
- **Local dev**: `docker compose up` ได้ทั้งระบบ · hot reload ของ app ที่แก้ใน studio
- **Test framework**: scenario test เขียนเป็น YAML/Go รันใน CI

---

## 4.3 สถาปัตยกรรมรองรับ workload สูง

### เป้าหมาย (ทดสอบจริงก่อนประกาศ)
| ตัวชี้วัด | 1 node | cluster (3–5 node) |
|---|---|---|
| step ที่ engine ประมวลผล | ≥ 200 / วินาที | ≥ 2,000 / วินาที |
| เรื่องเปิดพร้อมกัน (active) | 1 ล้าน | 10 ล้าน+ |
| ผู้ใช้ใช้งานพร้อมกัน | 2,000 | 20,000 |
| เปิดกล่องงาน / หน้าเรื่อง | p95 < 300 ms | p95 < 300 ms |
| timer คลาดเคลื่อน | < 10 วินาที | < 10 วินาที |

### หลักการ
1. **API ไม่เก็บ state** → เพิ่ม replica ได้แนวนอนหลัง load balancer
2. **แยก process ตามหน้าที่** (โค้ดชุดเดียว เลือกโหมดตอนรัน): `api` · `engine-worker` · `job-worker` · `scheduler` · `projector` (สร้างตาราง read/metric) — ขยายแต่ละส่วนตามภาระจริง
3. **Partition ตาม case id**: งานของเรื่องเดียวกันวิ่งที่ worker เดียวกันตามลำดับ (hash partition + lease) → ไม่ชนกัน ไม่ต้อง lock กว้าง
4. **CQRS**: เขียนลงตาราง write (instance, event) · อ่านจากตาราง read ที่ projector สร้าง (worklist, case list, metric) → หน้าจอเร็วโดยไม่แย่ง lock กับ engine
5. **Postgres ก่อน ขยายทีหลัง**: partition ตารางใหญ่ตามเดือน (event, history) · read replica ให้หน้ารายงาน · archive เรื่องที่ปิดแล้วไป cold storage · connection pool (pgbouncer)
6. **Outbox → event bus เมื่อจำเป็น**: เริ่มด้วย Postgres LISTEN/NOTIFY · โหลดสูงสลับเป็น NATS JetStream / Kafka ได้โดยไม่แก้ engine
7. **Backpressure + rate limit**: job queue มีเพดานต่อ connector · ระบบภายนอกช้าไม่ลากทั้งระบบ (circuit breaker)
8. **Cache**: definition ที่ compile แล้ว, data page, สิทธิ์ผู้ใช้ (in-memory + invalidate ผ่าน event)
9. **Observability**: OpenTelemetry (trace ข้าม api → engine → job → connector) · Prometheus metric · log แบบ structured · dashboard ระบบใน Admin
10. **HA**: ทุก process มีหลาย replica · scheduler/partition ใช้ lease ใน Postgres (ตัวไหนตายอีกตัวรับต่อใน < 30 วินาที) · zero-downtime deploy (migration แบบเข้ากันได้ย้อนหลัง)

### การพิสูจน์
- **Load test** (k6) ทุก release: สร้างเรื่อง, ส่งงาน, ค้นหา, dashboard ตามสัดส่วนการใช้งานจริง
- **Soak test** 24 ชม. ดู memory/connection รั่ว
- **Failover test**: ฆ่า node ระหว่างโหลด ต้องไม่มีงานหาย/ซ้ำ
- ผลวัดเก็บใน repo เทียบย้อนหลังทุก release

---

## 5. โมเดลข้อมูลใหม่ (เพิ่มจากของเดิม)

| ตาราง | หน้าที่ |
|---|---|
| `cm_app` / `cm_app_version` | application (กลุ่ม case type) + version รวม · แทน version ราย process |
| `cm_case_type` | stage, optional action, case-level SLA, child case rule, data type ที่ใช้ |
| `cm_data_type` | นิยามข้อมูล: field, type, required, ค่าเริ่มต้น, calculated expr, validation, ref ไปยัง data type อื่น / list |
| `cm_view` | หน้าจอ low-code (form / details / table / tabs) อ้าง field จาก data type |
| `cm_rule` | when rule, decision table, validation rule (มี version) |
| `cm_sla` | goal / deadline / passed-deadline + escalation action + business calendar |
| `cm_calendar` | วันทำการ เวลาทำการ วันหยุด |
| `cm_routing` | กติกาแจกงาน: queue, skill, workload, round-robin, ตัวแปร |
| `cm_queue` / `cm_queue_member` / `cm_skill` | work queue, สมาชิก, skill ของผู้ใช้ |
| `cm_delegation` | มอบอำนาจ / out-of-office |
| `cm_connector` | REST connector: URL, auth, request/response mapping, retry |
| `cm_case_data` | ข้อมูล case แบบ typed (jsonb ตาม data type) + index ช่องที่ค้นหาได้ |
| `cm_document` | เอกสาร: หมวด, ไฟล์ (object store), version, ผู้อัปโหลด |
| `cm_note` | ความเห็น, @mention |
| `cm_follow` / `cm_notification` | ติดตามเรื่อง, กล่องแจ้งเตือน |
| `cm_event` | event outbox (case.created, stage.entered, work.created/completed, sla.breached, error …) |
| `cm_metric_*` | สรุปรายช่วงเวลา: throughput, cycle time, backlog, SLA |
| `cm_incident` | instance ที่ error / ติด: สาเหตุ, step, retry count, สถานะการแก้ |
| `cm_report` / `cm_dashboard` | report ที่ผู้ใช้สร้างและ dashboard ที่บันทึกไว้ |
| `cm_saved_search` | ตัวกรองที่บันทึก |
| `cm_access_policy` | ABAC: เงื่อนไขการเห็น / แก้ case และ field |

---

## 6. รายการหน้าจอเป้าหมาย (ตาม persona)

### 6.1 App Studio (Designer)
1. **App overview**: case type, data, persona, สถานะ version, ผลการตรวจความครบถ้วน
2. **Case type designer**: แถบ stage แนวนอนแบบ Pega · ในแต่ละ stage มี process/step · ตั้ง optional action, child case, SLA ราย stage · สลับไปแก้ BPMN เต็มได้
3. **Data model designer**: ตาราง field, ชนิด, ความสัมพันธ์ (ER แบบย่อ), calculated field, validation
4. **View designer**: ลากวาง field จาก data model ลง region (form / details / table / tabs / section) · preview ตาม persona และขนาดจอ
5. **Rule designer**: decision table แบบ spreadsheet · when rule · ทดสอบ rule ในหน้าเดียว
6. **SLA & calendar designer**
7. **Routing & queue designer**
8. **Integration designer**: สร้าง REST connector, ทดลองยิง, map request/response กับ data model
9. **Persona & portal designer**: หน้าแรก, เมนู, widget ของแต่ละ persona
10. **Test run**: รันเรื่องจำลองทีละ step, ดู data และ token, บันทึกเป็น scenario test
11. **Versions & deploy**: diff, approval, publish, export/import app ข้าม environment

### 6.2 Workspace (ผู้ปฏิบัติงาน / ผู้ยื่น)
12. **My work**: งานของฉัน + **Get next work** + filter ที่บันทึกได้
13. **Case view**: header (stage bar), ข้อมูล, งานปัจจุบัน, optional action, เอกสาร, ความเห็น, เรื่องที่เกี่ยวข้อง, timeline
14. **Create case**: เลือก case type → ฟอร์มขั้นแรกทันที
15. **Search**: ค้นหาเต็ม + facet
16. **Notifications** + **Delegation settings**

### 6.3 Monitor (หัวหน้า / operations)
17. **Operations dashboard**: backlog, throughput, cycle time, SLA (on-time / at-risk / breached), workload รายคน/คิว · ช่วงเวลาเลือกได้ · ดูแบบ live
18. **Process monitor**: แผนภาพ flow + ตัวเลขบนแต่ละ step (ค้าง, เวลาเฉลี่ย, เกิน SLA) + heatmap คอขวด · คลิก step เพื่อดูเรื่องในนั้น
19. **Case explorer**: ค้นหาเรื่องตาม stage / step / SLA / ผู้รับผิดชอบ · bulk reassign / move / cancel
20. **Process inspector**: เปิดเรื่องเดียว เห็น token, ตัวแปร, งาน, timer · แก้ data / retry / ย้าย / ข้าม / ยกเลิก (ทุกอย่างบันทึก audit)
21. **Incident queue**: เรื่องที่ error / connector ล้ม / timer ค้าง · retry ทีละรายการหรือทั้งชุด
22. **SLA monitor**: เรื่องที่ใกล้เกินและเกินแล้ว แยก stage / step / queue
23. **Team workload**: งานรายคน, การแจกงานใหม่
24. **Reports**: report builder (เลือก field, กลุ่ม, chart) · export Excel/CSV/PDF · ตั้งส่งตามเวลา
25. **Process analytics**: variant ของเส้นทางที่เรื่องเดินจริง, rework loop, เทียบ version

### 6.4 Admin
26. Users / roles / skills / queues (sync จาก IAM)
27. Access policy (ABAC / field-level)
28. Tenants
29. Environments & connectors credentials
30. Import / export / migration (มีแล้ว)
31. System health: worker, timer, event lag, storage
32. Audit log (มีแล้ว)

---

## 7. แผนเป็นเฟส

| เฟส | ชื่อ | สัปดาห์ | ส่งมอบ |
|---|---|---|---|
| **0** | ฐานรากแพลตฟอร์ม | 1–2 | event outbox, Postgres เป็นค่าเริ่มต้น + test, IAM (OIDC), docker-compose, CI, i18n (EN/TH) |
| **1** | Case model | 3–6 | stage, data model, case data แบบ typed, calculated / validation, optional action, child / related case, case view ใหม่ (stage bar, timeline) |
| **2** | App Studio | 7–12 | App + app version, case type designer, data designer, view designer (ลากวาง), rule designer (decision table), integration designer, persona, test run |
| **3** | Work management | 13–15 | work queue, routing (skill / workload / round-robin), get next work, delegation, bulk action, SLA goal/deadline/escalation, business calendar, notification |
| **4** | Monitor & operations | 16–19 | metric rollup, operations dashboard, process monitor + heatmap, case explorer, process inspector, incident queue, SLA monitor, team workload (live ผ่าน SSE) |
| **5** | Collaboration & documents | 20–21 | เอกสาร (object store, หมวด, preview, version), ความเห็น/@mention, follow, ค้นหาเต็ม |
| **6** | Reports, analytics, enterprise | 22–24 | report builder + export + ตั้งเวลา, process analytics (variant), ABAC/field security, multi-tenant จริง, export/import app ข้าม environment, HA / backup / คู่มือติดตั้ง |

ลำดับนี้ทำตาม dependency: เฟส 1 ต้องมาก่อน studio (studio ออกแบบ stage/data) และ monitor ต้องมี event (เฟส 0) กับ SLA/queue (เฟส 3) ก่อน
ถ้าต้องการเห็นผลเร็ว **เฟส 4 แบบย่อ** (dashboard + process monitor + inspector + incident) ทำคู่ขนานกับเฟส 2 ได้ เพราะใช้แค่ event outbox จากเฟส 0

---

## 8. รายละเอียดงานรายเฟส

### เฟส 0: ฐานราก
- **Engine hardening (E1–E5 ในข้อ 4.1)**: revision lock, job executor + retry + incident, event log แยกตาราง, ตาราง message subscription, chaos test + benchmark
- **แยกโหมด process** (api / engine-worker / job-worker / scheduler / projector) + partition ตาม case id + lease (ข้อ 4.3)
- **OpenTelemetry + Prometheus** และชุด load test k6 ตั้งต้น
- **Event outbox** `cm_event` เขียนใน transaction เดียวกับการเปลี่ยน case · worker กระจายต่อ (SSE, notification, webhook, metric)
- **Postgres เป็นหลัก**: integration test ทั้งชุดรันบน Postgres (testcontainers) · memory store เหลือไว้ unit test
- **IAM**: OIDC (authorization code + PKCE) · map group → role · header auth เหลือเฉพาะ dev
- **i18n**: dictionary EN / TH · label ใน artifact ที่ออกแบบ (field, step) รองรับหลายภาษา
- **docker-compose** (api, web, postgres, minio) · Dockerfile · Jenkinsfile → QA
- **เกณฑ์รับงาน**: test ทั้งหมดผ่านบน Postgres · login ผ่าน OIDC จริง · `docker compose up` แล้วใช้งานได้

### เฟส 1: Case model
- **Stage layer**: case type = รายการ stage (primary / alternate / resolution) · stage มี process ≥ 1 ตัว (BPMN เดิม), เงื่อนไขเข้า/ข้าม, SLA ราย stage · ย้าย stage ด้วย action หรืออัตโนมัติเมื่อ process ใน stage จบ
- **Data model**: data type + field (text, number, money, date, bool, enum, reference, list, attachment, user, org) · case มี data type หลัก · ฟอร์มอ้าง field ไม่เก็บ key อิสระ
- **Calculated field / validation rule** ใช้ expression ชุดเดียวกับ engine
- **Optional action**: ปุ่มบนหน้า case เปิด process ย่อยหรือ step เดี่ยว (เช่น "ขอข้อมูลเพิ่ม", "แก้ไขผู้รับผิดชอบ")
- **Child / related case**: สร้างลูกจาก step หรือจาก action · รอลูก all / any · ผูกเรื่องที่เกี่ยวข้องได้
- **Case view ใหม่**: stage bar ด้านบน · tab: Overview / Data / Documents / Notes / Related / Timeline
- **Migration**: เรื่องที่มีอยู่ map เข้า stage เดียว ("Default")
- **เกณฑ์รับงาน**: สร้าง case type 4 stage มี alternate stage และ optional action · child case รอครบแล้ว parent เดินต่อ · ข้อมูล typed ตรวจ validation ฝั่ง server

### เฟส 2: App Studio
- โครง studio แบบ Carbon สองโหมด Business / Developer ตามข้อ 4.2 (left nav: Case types / Data / Views / Rules / Integrations / Personas / Settings · right panel: properties)
- **Case type designer**: ลาก stage, เพิ่ม step (user / approval / automation / sub-process / child case), กำหนด condition, เปิด BPMN เต็มได้
- **View designer**: palette field จาก data model · region / section / tab / repeating table · กฎ visible / required / read-only ตามเงื่อนไขและ persona · preview desktop / tablet / mobile
- **Rule designer**: decision table (เงื่อนไข × ผลลัพธ์, ตรวจ overlap / gap), when rule, ทดสอบด้วยข้อมูลตัวอย่าง
- **Integration designer**: REST connector (method, URL, header, auth แบบอ้าง secret), ทดลองยิง, map JSON ↔ data model, retry policy, timeout
- **Persona designer**: หน้าแรก, เมนู, widget
- **Test run**: สร้าง case จำลองใน sandbox, กดเดินทีละ step, ดู data / token / rule ที่ถูกเรียก, บันทึกเป็น scenario test ใช้รันใน CI
- **App version**: publish ทั้ง app ทีเดียว · diff ระดับ artifact · export `.cmapp`
- **Developer tooling**: CLI `cm` (pull/push app เป็นไฟล์ใน git), OpenAPI, SDK Go/TS, custom widget
- **เกณฑ์รับงาน**: ผู้ใช้ที่ไม่ใช่นักพัฒนาสร้าง app คำขอจัดซื้อครบ (3 stage, 2 view, decision table, 1 connector) โดยไม่แตะ BPMN XML หรือโค้ด ภายใน 1 ชั่วโมงตามบทสาธิต

### เฟส 3: Work management
- **Work queue** + สมาชิก · **routing**: ไปคน / queue / skill / workload ต่ำสุด / round-robin / ตาม expression
- **Get next work**: ดึงงานถัดไปตาม urgency และ SLA จากคิวที่ตัวเองอยู่
- **Urgency** (0–100) คำนวณจาก SLA + ค่าที่ตั้ง
- **SLA**: goal → deadline → passed-deadline · escalation action: แจ้งเตือน, เพิ่ม urgency, ย้ายคิว, มอบหมายหัวหน้า, เริ่ม process
- **Business calendar**: เวลาทำการ วันหยุด (นับ SLA ตามวันทำการ)
- **Delegation / out-of-office** · **bulk action** (reassign, transfer, cancel)
- **Notification**: in-app + email (template) · ตั้งค่ารายผู้ใช้
- **เกณฑ์รับงาน**: งานแจกตาม workload ได้สมดุล ±1 · SLA นับข้ามวันหยุดถูก · escalation ทำงานตามเวลาจริงเมื่อเลื่อนนาฬิกาใน test

### เฟส 4: Monitor & operations
- **Metric rollup worker** จาก event: backlog, arrivals, completions, cycle time (p50/p90), เวลารอ, SLA status ราย app / case type / stage / step / queue / คน
- **Operations dashboard** (Carbon charts): KPI tile, trend, SLA donut, top คอขวด · filter ช่วงเวลา / case type / version · อัปเดตแบบ live (SSE)
- **Process monitor**: แผนภาพ BPMN / stage + overlay ตัวเลข + heatmap · คลิก step → case explorer ที่กรองไว้แล้ว
- **Case explorer**: ตาราง Carbon ที่ filter / sort / เลือกหลายรายการได้ · bulk action
- **Process inspector**: token tree, ตัวแปร (แก้ได้พร้อมเหตุผล), work item, timer, connector call ล่าสุด · action: retry, skip, move, cancel, เพิ่ม timer
- **Incident queue**: จับ error ของ connector / script / timer / migration · สถานะ open → resolved · retry ทีละรายการหรือทั้งชุด · แจ้งเตือนผู้ดูแล
- **SLA monitor** + **team workload**
- **เกณฑ์รับงาน**: ข้อมูล 10,000 เรื่อง dashboard เปิดภายใน 2 วินาที · ทำให้ connector ล้มแล้วเรื่องเข้า incident queue, retry แล้วเดินต่อ · ตัวเลขบน process monitor ตรงกับ query ตรงจากฐานข้อมูล

### เฟส 5: Collaboration & documents
- เอกสาร: อัปโหลดลากวาง, หมวดตาม case type, version, preview (PDF / รูป), เก็บใน object store (S3 / MinIO)
- ความเห็นพร้อม @mention, แนบไฟล์, แจ้งเตือนคนที่ถูก mention และคนที่ follow
- ค้นหาเต็มด้วย Postgres FTS (ไทย + อังกฤษ) ครอบข้อมูล case, ความเห็น และชื่อเอกสาร
- **เกณฑ์รับงาน**: ค้นหาคำภาษาไทยในความเห็นเจอ · ไฟล์ 50 MB อัปโหลดได้ · สิทธิ์ดูเอกสารเป็นไปตาม policy

### เฟส 6: Reports, analytics, enterprise
- Report builder: เลือก case type → field → group / aggregate → table / chart · บันทึก / แชร์ / ตั้งส่งอีเมล · export XLSX / CSV / PDF
- Process analytics: variant ของเส้นทาง, rework, เวลาต่อ variant, เทียบ version
- ABAC + field-level security (ซ่อน / อ่านอย่างเดียวตามเงื่อนไข)
- Multi-tenant จริง (row-level security ใน Postgres)
- Export / import app ข้าม environment พร้อมตัวแปรต่อ environment
- HA + failover test, CQRS read model ครบ, partition ตารางใหญ่, archive, load/soak test ตามเป้าข้อ 4.3, backup / restore, คู่มือติดตั้ง
- **เกณฑ์รับงาน**: ผู้ใช้สร้าง report ได้เองโดยไม่ต้องเขียน SQL · tenant A มองไม่เห็นข้อมูล tenant B (มี test) · ติดตั้งใหม่จากคู่มือได้ภายใน 1 ชั่วโมง

---

## 9. มาตรฐานที่ใช้ทุกเฟส

- **UI**: IBM Carbon (g10) · component ของเราเอง ไม่ใช้ native select / choose file · ไม่มีตัวโหลดหมุน · ไม่มีข้อความสอนวิธีใช้บนจอ · ไม่มีป้ายตัวเลขมุมโค้ง
- **ID**: CUID ทั้งหมดรวมใน URL · URL ตัวพิมพ์เล็ก
- **API**: REST + JSON · PUT/DELETE ส่งผ่าน POST + `X-HTTP-Method-Override` ได้
- **ข้อมูลลูกค้า**: ห้ามขึ้น repo · ชุดตัวอย่างใน repo เป็นงานกลาง (เช่น คำขอจัดซื้อ)
- **คุณภาพ**: ทุกฟีเจอร์มี test ฝั่ง Go · เปิดดูทุกหน้าในเบราว์เซอร์ก่อนส่ง · deploy ผ่าน Jenkins → QA ก่อน UAT

---

## 10. ความเสี่ยงและวิธีรับมือ

| ความเสี่ยง | ผลกระทบ | วิธีรับมือ |
|---|---|---|
| ขอบเขตใหญ่ (studio คือ product ในตัวเอง) | เลื่อน | ทำ "บทสาธิต" ของแต่ละเฟสก่อน แล้วค่อยเติมให้ครบ · ตัดของที่ไม่อยู่ในเกณฑ์รับงาน |
| Stage layer ชนกับ BPMN ที่มีอยู่ | engine ซับซ้อน | stage คุม process เป็นกล่องดำ ไม่แก้ token engine · มี migration ให้ของเดิม |
| Performance ของ monitor | dashboard ช้า | metric rollup + index · ทดสอบโหลด 10k / 100k เรื่องตั้งแต่เฟส 4 |
| View designer ลากวางทำยาก | UX ไม่ดี | ใช้ dnd-kit · ยึด region template แบบ Constellation แทนวางอิสระ |
| ภาษาไทยใน FTS | ค้นไม่เจอ | ตัดคำด้วย ICU ในฝั่ง Go ก่อนบันทึก |

---

## 11. สิ่งที่ต้องเคาะก่อนเริ่ม

1. **ลำดับเฟส**: ตามตารางข้อ 7 หรือดึง monitor (เฟส 4 แบบย่อ) มาทำคู่กับเฟส 2
2. ~~Workflow engine~~ **เคาะแล้ว: ทำ engine เอง** (ข้อ 4.1) · เฟส 0 ขยายเป็น 3 สัปดาห์เพื่อรวม engine hardening
3. **Object store**: MinIO ในตัว หรือ object store ที่องค์กรมีอยู่
4. **IAM**: OIDC มาตรฐาน (แนะนำ ใช้ได้กับทุกลูกค้า) หรือผูก IAM ภายในองค์กรโดยตรง
5. **ภาษา**: EN เป็นหลัก + TH ในเฟส 0 (แนะนำ) หรือ EN อย่างเดียว

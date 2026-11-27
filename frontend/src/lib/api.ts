// API client. In development the caller identity is sent as X-User / X-Roles
// (chosen on the sign-in page). In production an identity gateway (OIDC /
// IAM proxy) sets these headers and the stored dev session is ignored.
export type Session = { id: string; name: string; roles: string[] };

const KEY = "cm.session";

export function getSession(): Session | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = localStorage.getItem(KEY);
    return raw ? (JSON.parse(raw) as Session) : null;
  } catch {
    return null;
  }
}

export function setSession(s: Session | null) {
  try {
    if (s) localStorage.setItem(KEY, JSON.stringify(s));
    else localStorage.removeItem(KEY);
  } catch {}
  window.dispatchEvent(new Event("cm-session"));
}

// Access token from the OIDC sign-in (sessionStorage), if any.
function bearer(): string | null {
  try {
    const raw = sessionStorage.getItem("cm.token");
    if (!raw) return null;
    const t = JSON.parse(raw) as { access: string; exp: number };
    return t.exp * 1000 > Date.now() ? t.access : null;
  } catch {
    return null;
  }
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

async function call<T>(method: string, path: string, body?: unknown, raw?: BodyInit): Promise<T> {
  const s = getSession();
  const headers: Record<string, string> = {};
  const token = bearer();
  if (token) headers["Authorization"] = `Bearer ${token}`;
  else if (s) {
    headers["X-User"] = s.id;
    headers["X-User-Name"] = encodeURIComponent(s.name);
    headers["X-Roles"] = s.roles.join(",");
  }
  let init: RequestInit = { method, headers };
  if (raw) init.body = raw;
  else if (body !== undefined) {
    headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(body);
  }
  // Some WAFs block PUT/DELETE: tunnel them through POST.
  if (method === "PUT" || method === "DELETE") {
    headers["X-HTTP-Method-Override"] = method;
    init.method = "POST";
  }
  const r = await fetch(path, init);
  const text = await r.text();
  const data = text ? JSON.parse(text) : null;
  if ((r.status === 401 || (r.status === 403 && token === null && sessionStorage.getItem("cm.token"))) && s) {
    // expired or rejected session: sign in again (once, not from the login page)
    setSession(null);
    if (!location.pathname.startsWith("/login") && !location.pathname.startsWith("/auth/")) location.assign("/login");
  }
  if (!r.ok) throw new ApiError(r.status, data?.error ?? r.statusText);
  return data as T;
}

export const api = {
  get: <T,>(p: string) => call<T>("GET", p),
  post: <T,>(p: string, b?: unknown) => call<T>("POST", p, b ?? {}),
  put: <T,>(p: string, b: unknown) => call<T>("PUT", p, b),
  upload: <T,>(p: string, f: FormData) => call<T>("POST", p, undefined, f),
  del: <T,>(p: string) => call<T>("DELETE", p),
};

// ---- types mirrored from the Go API ----
export type ProcessDef = { id: string; key: string; name: string; description?: string; category?: string; parentKey?: string };
export type Version = {
  id: string; processKey: string; no: number; status: string; fiscalYear?: number; bpmn: string; note?: string;
  createdBy: string; approvedBy?: string; publishedAt?: string; createdAt: string; clonedFrom?: string;
};
export type Binding = {
  versionId?: string; nodeId: string; ui: "form" | "external" | "none"; formKey?: string; screenKey?: string;
  url?: string; embed?: boolean; role?: string; sla?: string; enabled?: boolean; condition?: string;
  route?: Routing; goal?: string; urgency?: number; escalate?: Escalation[];
};
export type Step = { nodeId: string; name: string; type: string; lane?: string; human: boolean; binding: Binding };
export type Field = {
  key: string; label: string; type: string; required?: boolean; min?: number; max?: number;
  options?: { value: string; label: string }[]; visibleIf?: string; readOnly?: boolean; width?: number;
  columns?: Field[]; source?: string; help?: string;
};
export type Form = { id?: string; key: string; name: string; version?: number; schema: { fields: Field[] } };
export type Widget = { width: number; widget: string; url?: string; text?: string };
export type Screen = { id?: string; key: string; name: string; kind: string; roles?: string[]; layout: { rows: { cols: Widget[] }[] } };
export type Token = { id: string; node: string; wait?: string; scope?: string };
export type History = { at: string; node: string; event: string; token: string; by?: string };
export type Instance = { id: string; status: string; vars: Record<string, unknown>; tokens: Token[] | null; history: History[]; error?: string };
export type Case = {
  id: string; number: string; processKey: string; versionId: string; title: string; fiscalYear?: number;
  parentId?: string; status: string; instance: Instance; stage?: string; relatedIds?: string[]; createdBy: string; createdAt: string; updatedAt: string;
};
export type Work = {
  id: string; instanceId: string; tokenId: string; node: string; name: string; role?: string; assignee?: string;
  ui: "form" | "external" | "none"; formKey?: string; url?: string; embed?: boolean; screenKey?: string;
  input?: Record<string, unknown>; sla?: string; dueAt?: string; caseId: string; caseNumber: string; caseTitle: string;
  status: string; createdAt: string; completedBy?: string; completedAt?: string;
  queue?: string; urgency?: number; goalAt?: string; delegatedFrom?: string;
};
export type Audit = { at: string; actor: string; action: string; entity: string; ref: string; data?: Record<string, unknown> };

// ---- case model ----
export type StageDef = { key: string; name: string; kind: "primary" | "alternate" | "resolution"; steps: string[]; entry?: string; sla?: string };
export type DataFieldDef = {
  key: string; label: string; type: string; required?: boolean; requiredIn?: string[];
  options?: { value: string; label: string }[]; calc?: string; searchable?: boolean; summary?: boolean; help?: string;
};
export type DataRuleDef = { when?: string; check: string; message: string };
export type ActionDef = {
  key: string; label: string; kind: "update" | "stage" | "child"; target?: string; formKey?: string;
  stages?: string[]; roles?: string[]; pass?: string[]; confirm?: boolean;
};
export type CaseModel = { stages: StageDef[]; data: DataFieldDef[]; rules?: DataRuleDef[]; actions: ActionDef[]; lifecycle?: Lifecycle; decisions?: DecisionTable[]; connectors?: Connector[] };
export type StageView = { key: string; name: string; kind: string; state: "done" | "current" | "upcoming" | "skipped"; entered?: string; left?: string; dueAt?: string };
export type ActionView = ActionDef & { allowed: boolean };
export type EventRec = { seq: number; at: string; type: string; node?: string; token?: string; by?: string; data?: Record<string, unknown> };

// ---- studio ----
export type LCStep = {
  key: string; name: string; kind: "task" | "approval" | "automation" | "decision" | "wait" | "child" | "timer";
  role?: string; form?: string; screen?: string; url?: string; sla?: string; when?: string; multi?: string;
  onReject?: string; decisionField?: string; connector?: string; decision?: string; message?: string; caseType?: string;
};
export type LCStage = { key: string; name: string; kind: "primary" | "alternate" | "resolution"; sla?: string; steps: LCStep[] };
export type Lifecycle = { stages: LCStage[]; detached?: boolean };
export type DTColumn = { key: string; label: string };
export type DecisionTable = { key: string; name: string; inputs: DTColumn[]; outputs: DTColumn[]; rows: { when: string[]; then: string[]; note?: string }[]; hit?: string };
export type Connector = {
  key: string; name: string; method: string; url: string; headers?: Record<string, string>; body?: Record<string, string>;
  response?: Record<string, string>; timeout?: number; retries?: number;
};

// ---- work management ----
export type OutOfOffice = { from?: string; to?: string; delegate?: string };
export type UserProfile = { id: string; name: string; email?: string; roles: string[]; skills?: string[]; capacity?: number; active: boolean; ooo?: OutOfOffice; lastSeen?: string };
export type Queue = { key: string; name: string; roles?: string[]; members?: string[]; supervisors?: string[] };
export type Calendar = { enabled: boolean; days: number[]; start: string; end: string; holidays?: string[]; tz?: string };
export type Notification = { id: string; at: string; kind: string; title: string; link?: string; read: boolean };
export type Routing = { mode?: string; queue?: string; user?: string; skill?: string; expr?: string };
export type Escalation = { at: "goal" | "deadline"; do: "notify" | "urgency" | "reassign" | "queue"; target?: string; amount?: number };
export type Incident = { id: string; caseId: string; caseNumber: string; processKey: string; tokenId: string; node: string; nodeName: string; message: string; status: string; createdAt: string; resolvedAt?: string; resolvedBy?: string; resolution?: string };

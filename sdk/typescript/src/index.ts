// TypeScript client for the Case Management API (browser and Node 18+).
//
//   const cm = new CaseManagement({ baseUrl: "https://cases.example.com", token });
//   const c = await cm.startCase({ processKey: "purchase-request", vars: { amount: 1000 } });

export type StartInput = { processKey: string; fiscalYear?: number; title?: string; vars?: Record<string, unknown>; parentId?: string };
export type Case = { id: string; number: string; processKey: string; title: string; status: string; stage?: string; instance: { vars: Record<string, unknown> } };
export type Work = { id: string; caseId: string; caseNumber: string; node: string; name: string; role?: string; assignee?: string; status: string; dueAt?: string; formKey?: string };

export class ApiError extends Error {
  constructor(public status: number, message: string) { super(message); }
}

export type Options = {
  baseUrl: string;
  /** OIDC access token, or a function returning a fresh one. */
  token?: string | (() => string | Promise<string>);
  /** Development servers only. */
  devUser?: { id: string; roles: string[] };
  fetch?: typeof fetch;
};

export class CaseManagement {
  constructor(private o: Options) {}

  private async req<T>(method: string, path: string, body?: unknown): Promise<T> {
    const h: Record<string, string> = { "Content-Type": "application/json" };
    const tok = typeof this.o.token === "function" ? await this.o.token() : this.o.token;
    if (tok) h.Authorization = `Bearer ${tok}`;
    else if (this.o.devUser) { h["X-User"] = this.o.devUser.id; h["X-Roles"] = this.o.devUser.roles.join(","); }
    const f = this.o.fetch ?? fetch;
    const r = await f(this.o.baseUrl.replace(/\/$/, "") + path, { method, headers: h, body: body === undefined ? undefined : JSON.stringify(body) });
    const text = await r.text();
    const data = text ? JSON.parse(text) : undefined;
    if (!r.ok) throw new ApiError(r.status, data?.error ?? r.statusText);
    return data as T;
  }

  startCase(input: StartInput) { return this.req<Case>("POST", "/api/cases", input); }
  async getCase(id: string) { return (await this.req<{ case: Case }>("GET", `/api/cases/${id}`)).case; }
  myWork() { return this.req<Work[]>("GET", "/api/work"); }
  claim(workId: string) { return this.req<Work>("POST", `/api/work/${workId}/claim`); }
  completeWork(workId: string, data: Record<string, unknown>) { return this.req<Case>("POST", `/api/work/${workId}/complete`, data); }
  runAction(caseId: string, action: string, data: Record<string, unknown> = {}) { return this.req<unknown>("POST", `/api/cases/${caseId}/actions/${action}`, data); }
  async message(name: string, target: { caseId?: string; correlationKey?: string }, vars?: Record<string, unknown>) {
    return (await this.req<{ matched: number }>("POST", "/api/messages", { name, ...target, vars })).matched;
  }
  signal(name: string, vars?: Record<string, unknown>) { return this.req<unknown>("POST", "/api/signals", { name, vars }); }
  /** Completes an external-screen step with the signed callback URL the platform passed to your page. */
  static async callback(callbackUrl: string, data: Record<string, unknown>) {
    const r = await fetch(callbackUrl, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(data) });
    if (!r.ok) throw new ApiError(r.status, (await r.json().catch(() => ({}))).error ?? r.statusText);
    return r.json();
  }
}

// OpenID Connect sign-in (authorization code + PKCE, public client).
// Enabled when NEXT_PUBLIC_OIDC_ISSUER and NEXT_PUBLIC_OIDC_CLIENT_ID are set
// at build time; works with any standard provider.
import { Session, setSession } from "./api";

export const oidc = {
  issuer: (process.env.NEXT_PUBLIC_OIDC_ISSUER ?? "").replace(/\/$/, ""),
  clientId: process.env.NEXT_PUBLIC_OIDC_CLIENT_ID ?? "",
  scope: process.env.NEXT_PUBLIC_OIDC_SCOPE ?? "openid profile email",
  rolesClaim: process.env.NEXT_PUBLIC_OIDC_ROLES_CLAIM ?? "roles",
  devLogin: process.env.NEXT_PUBLIC_DEV_LOGIN !== "0",
};
export const oidcEnabled = () => !!(oidc.issuer && oidc.clientId);

const TOKEN = "cm.token";

export function getToken(): string | null {
  try {
    const raw = sessionStorage.getItem(TOKEN);
    if (!raw) return null;
    const t = JSON.parse(raw) as { access: string; exp: number };
    return t.exp * 1000 > Date.now() ? t.access : null;
  } catch {
    return null;
  }
}

async function discovery() {
  const r = await fetch(`${oidc.issuer}/.well-known/openid-configuration`);
  return (await r.json()) as { authorization_endpoint: string; token_endpoint: string; end_session_endpoint?: string };
}

const b64url = (b: ArrayBuffer | Uint8Array) =>
  btoa(String.fromCharCode(...new Uint8Array(b as ArrayBuffer))).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");

function random(n = 32) {
  const a = new Uint8Array(n);
  crypto.getRandomValues(a);
  return b64url(a);
}

const redirectUri = () => `${location.origin}/auth/callback`;

export async function signIn(returnTo = "/") {
  const d = await discovery();
  const verifier = random(48);
  const state = random(16);
  const challenge = b64url(await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier)));
  sessionStorage.setItem("cm.pkce", JSON.stringify({ verifier, state, returnTo }));
  const q = new URLSearchParams({
    response_type: "code", client_id: oidc.clientId, redirect_uri: redirectUri(), scope: oidc.scope,
    state, code_challenge: challenge, code_challenge_method: "S256",
  });
  location.assign(`${d.authorization_endpoint}?${q}`);
}

function claims(jwt: string): Record<string, any> {
  const p = jwt.split(".")[1].replace(/-/g, "+").replace(/_/g, "/");
  return JSON.parse(decodeURIComponent(escape(atob(p))));
}

function rolesFrom(c: Record<string, any>): string[] {
  let cur: any = c;
  for (const part of oidc.rolesClaim.split(".")) cur = cur?.[part];
  if (Array.isArray(cur)) return cur.map((x) => String(x).replace(/^\//, ""));
  if (typeof cur === "string") return cur.split(/[\s,]+/).filter(Boolean);
  return [];
}

/** Completes the redirect: exchanges the code and stores the session. */
export async function handleCallback(): Promise<string> {
  const q = new URLSearchParams(location.search);
  const saved = JSON.parse(sessionStorage.getItem("cm.pkce") ?? "{}");
  if (q.get("error")) throw new Error(q.get("error_description") ?? q.get("error")!);
  if (!saved.state || q.get("state") !== saved.state) throw new Error("Sign-in state mismatch");
  const d = await discovery();
  const r = await fetch(d.token_endpoint, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({
      grant_type: "authorization_code", code: q.get("code") ?? "", redirect_uri: redirectUri(),
      client_id: oidc.clientId, code_verifier: saved.verifier,
    }),
  });
  const tok = await r.json();
  if (!r.ok) throw new Error(tok.error_description ?? tok.error ?? "Token request failed");
  sessionStorage.removeItem("cm.pkce");
  const access = claims(tok.access_token);
  const id = tok.id_token ? claims(tok.id_token) : access;
  sessionStorage.setItem(TOKEN, JSON.stringify({ access: tok.access_token, exp: access.exp, idToken: tok.id_token }));
  const s: Session = {
    id: id.preferred_username ?? id.sub,
    name: id.name ?? id.preferred_username ?? id.sub,
    roles: rolesFrom(access).length ? rolesFrom(access) : rolesFrom(id),
  };
  setSession(s);
  return saved.returnTo ?? "/";
}

export async function signOut() {
  let idToken: string | undefined;
  try { idToken = JSON.parse(sessionStorage.getItem(TOKEN) ?? "{}").idToken; } catch {}
  sessionStorage.removeItem(TOKEN);
  setSession(null);
  if (oidcEnabled()) {
    const d = await discovery();
    if (d.end_session_endpoint) {
      const q = new URLSearchParams({ post_logout_redirect_uri: `${location.origin}/login`, client_id: oidc.clientId });
      if (idToken) q.set("id_token_hint", idToken);
      location.assign(`${d.end_session_endpoint}?${q}`);
      return;
    }
  }
  location.assign("/login");
}

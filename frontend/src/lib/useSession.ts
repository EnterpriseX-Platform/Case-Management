"use client";
import { useEffect, useState } from "react";
import { getSession, Session } from "./api";

export function useSession() {
  const [s, setS] = useState<Session | null | undefined>(undefined);
  useEffect(() => {
    const f = () => setS(getSession());
    f();
    window.addEventListener("cm-session", f);
    window.addEventListener("storage", f);
    return () => { window.removeEventListener("cm-session", f); window.removeEventListener("storage", f); };
  }, []);
  return s;
}

export const has = (s: Session | null | undefined, role: string) =>
  !!s && (s.roles.includes(role) || s.roles.includes("cm:admin"));

"use client";
import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { has, useSession } from "@/lib/useSession";

// Landing: send each role to its main screen.
export default function Home() {
  const s = useSession();
  const r = useRouter();
  useEffect(() => {
    if (!s) return;
    r.replace("/home"); // everyone starts in the Workspace; other areas are one click away in the switcher
  }, [s, r]);
  return null;
}

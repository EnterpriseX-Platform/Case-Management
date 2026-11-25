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
    if (has(s, "cm:admin")) r.replace("/admin");
    else if (has(s, "cm:supervisor")) r.replace("/supervisor");
    else if (has(s, "cm:designer") || has(s, "cm:approver")) r.replace("/designer");
    else r.replace("/work");
  }, [s, r]);
  return null;
}

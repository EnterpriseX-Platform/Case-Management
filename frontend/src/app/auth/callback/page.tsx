"use client";
import { useEffect, useState } from "react";
import { handleCallback } from "@/lib/oidc";
import { Err } from "@/components/ui";

export default function Callback() {
  const [e, setE] = useState<string>();
  useEffect(() => {
    handleCallback().then((to) => location.replace(to)).catch((x) => setE(x.message));
  }, []);
  return <div className="login"><Err e={e} /></div>;
}

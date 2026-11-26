"use client";
// Case types now live in the Studio.
import { useRouter } from "next/navigation";
import { useEffect } from "react";

export default function Designer() {
  const r = useRouter();
  useEffect(() => r.replace("/studio"), [r]);
  return null;
}

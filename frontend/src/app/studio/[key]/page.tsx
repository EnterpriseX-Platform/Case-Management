"use client";
// /studio/<app> opens the latest version.
import { useParams, useRouter } from "next/navigation";
import { useEffect } from "react";
import { api, Version } from "@/lib/api";

export default function Latest() {
  const { key } = useParams<{ key: string }>();
  const r = useRouter();
  useEffect(() => {
    api.get<Version[]>(`/api/processes/${key}/versions`).then((vs) => vs?.[0] && r.replace(`/studio/${key}/${vs[0].id}/lifecycle`));
  }, [key, r]);
  return null;
}

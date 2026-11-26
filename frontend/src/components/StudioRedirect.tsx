"use client";
// Old /designer/versions/<id>/... links open the same editor inside the Studio.
import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { api, Version } from "@/lib/api";

export function useStudioRedirect(id: string, inStudio: boolean, section: string) {
  const r = useRouter();
  useEffect(() => {
    if (inStudio || !id) return;
    api.get<Version>(`/api/versions/${id}`).then((v) => r.replace(`/studio/${v.processKey}/${id}/${section}`)).catch(() => {});
  }, [id, inStudio, section, r]);
}

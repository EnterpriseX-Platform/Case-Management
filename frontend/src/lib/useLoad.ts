"use client";
import { useCallback, useEffect, useState } from "react";

/** Loads data; keeps previous data while reloading (no spinners). */
export function useLoad<T>(fn: () => Promise<T>, deps: unknown[] = []) {
  const [data, setData] = useState<T | undefined>();
  const [error, setError] = useState<string>();
  const load = useCallback(() => {
    fn().then((d) => { setData(d); setError(undefined); }).catch((e) => setError(String(e.message ?? e)));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
  useEffect(() => { load(); }, [load]);
  return { data, error, reload: load, setData };
}

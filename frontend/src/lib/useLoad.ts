"use client";
import { useCallback, useEffect, useRef, useState } from "react";

/**
 * Loads data; keeps previous data while reloading (no spinners). Only the
 * latest request may set the result, so a slow earlier request (e.g. before
 * filters were read from the URL) never overwrites a newer one.
 */
export function useLoad<T>(fn: () => Promise<T>, deps: unknown[] = []) {
  const [data, setData] = useState<T | undefined>();
  const [error, setError] = useState<string>();
  const seq = useRef(0);
  const load = useCallback(() => {
    const n = ++seq.current;
    fn()
      .then((d) => { if (n === seq.current) { setData(d); setError(undefined); } })
      .catch((e) => { if (n === seq.current) setError(String(e.message ?? e)); });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
  useEffect(() => { load(); }, [load]);
  return { data, error, reload: load, setData };
}

"use client";

import { useCallback, useEffect, useState } from "react";
import { useAuth } from "@/lib/auth-context";
import { getSeriesProgress, SeriesProgress } from "@/lib/api";

// Several components on the series page read the same progress; share one
// request per series and let any of them trigger a refresh.
const cache = new Map<string, Promise<SeriesProgress>>();
const listeners = new Map<string, Set<() => void>>();

export function invalidateSeriesProgress(seriesId: string) {
  cache.delete(seriesId);
  listeners.get(seriesId)?.forEach((fn) => fn());
}

export function useSeriesProgress(seriesId?: string) {
  const { token, isLoading } = useAuth();
  const [data, setData] = useState<SeriesProgress | null>(null);
  const [version, setVersion] = useState(0);

  useEffect(() => {
    if (!seriesId) return;
    const bump = () => setVersion((v) => v + 1);
    const set = listeners.get(seriesId) ?? new Set();
    set.add(bump);
    listeners.set(seriesId, set);
    return () => {
      set.delete(bump);
    };
  }, [seriesId]);

  useEffect(() => {
    if (isLoading || !seriesId) return;
    if (!token) {
      setData(null);
      return;
    }
    let active = true;
    let p = cache.get(seriesId);
    if (!p) {
      p = getSeriesProgress(token, seriesId);
      cache.set(seriesId, p);
      p.catch(() => cache.delete(seriesId));
    }
    p.then((d) => active && setData(d)).catch(() => {});
    return () => {
      active = false;
    };
  }, [seriesId, token, isLoading, version]);

  const refresh = useCallback(() => {
    if (seriesId) invalidateSeriesProgress(seriesId);
  }, [seriesId]);

  return { progress: data, refresh, token };
}

import { useState, useEffect, useCallback, useRef } from "react";
import { Search, Loader2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useApp } from "@/hooks/useApp";
import type { style } from "@/lib/wailsjs/go/models";

const PAGE_SIZE = 50;

interface Props {
  onSelectSample: (id: number) => void;
  activeId?: number | null;
  novelId?: number;
}

export default function StyleSampleList({
  onSelectSample,
  activeId,
  novelId = 0,
}: Props) {
  const app = useApp();
  const { t } = useTranslation();
  const [samples, setSamples] = useState<style.Sample[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [hasMore, setHasMore] = useState(true);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState("");
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const loadingRef = useRef(false);
  const searchRef = useRef("");
  const listRef = useRef<HTMLDivElement>(null);
  const loadPageRef = useRef<(p: number, q: string) => void>(null as any);

  // eslint-disable-next-line react-hooks/refs
  loadPageRef.current = async (p: number, q: string) => {
    if (loadingRef.current) return;
    loadingRef.current = true;
    setLoading(true);
    try {
      const result = await app.ListStyleSamples({
        novel_id: novelId,
        page: p,
        size: PAGE_SIZE,
        search: q,
      });
      if (result?.items) {
        setSamples((prev) =>
          p === 1 ? result.items : [...prev, ...result.items],
        );
        setTotal(result.total);
        setHasMore(result.page < result.total_pages);
      }
    } catch {
      // ignore
    } finally {
      setLoading(false);
      loadingRef.current = false;
    }
  };

  useEffect(() => {
    loadPageRef.current?.(1, "");
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // novelId 变化时重置列表
  useEffect(() => {
    setSamples([]);
    setPage(1);
    setHasMore(true);
    setSearch("");
    searchRef.current = "";
    loadPageRef.current?.(1, "");
  }, [novelId]); // eslint-disable-line react-hooks/exhaustive-deps

  // 搜索防抖 300ms
  useEffect(() => {
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => {
      if (searchRef.current !== search) {
        searchRef.current = search;
        setSamples([]);
        setPage(1);
        setHasMore(true);
        loadPageRef.current?.(1, search);
      }
    }, 300);
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [search]);

  const handleScroll = useCallback(() => {
    if (!listRef.current || !hasMore || loading) return;
    const { scrollTop, scrollHeight, clientHeight } = listRef.current;
    if (scrollHeight - scrollTop - clientHeight < 80) {
      const next = page + 1;
      setPage(next);
      loadPageRef.current?.(next, searchRef.current);
    }
  }, [hasMore, loading, page]);

  return (
    <>
      <div className="flex items-center justify-between px-3 py-2.5 border-b">
        <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          {t("styleSample.samples")} ({total})
        </span>
      </div>
      <div className="px-2 py-1.5 border-b">
        <div className="relative">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-3 h-3 text-muted-foreground" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t("styleSample.searchPlaceholder")}
            className="w-full h-7 rounded-md border bg-background pl-7 pr-2 text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>
      </div>
      <div
        ref={listRef}
        onScroll={handleScroll}
        className="flex-1 overflow-y-auto overscroll-contain"
      >
        {samples.length === 0 && loading ? (
          <div className="flex items-center justify-center py-8 text-xs text-muted-foreground">
            {t("styleSample.loading")}
          </div>
        ) : samples.length === 0 ? (
          <div className="flex items-center justify-center py-8 text-xs text-muted-foreground">
            {search ? t("styleSample.noMatching") : t("styleSample.noSamples")}
          </div>
        ) : (
          <>
            {samples.map((s) => (
              <button
                key={s.id}
                onClick={() => onSelectSample(s.id)}
                className={`relative w-full flex flex-col px-3 py-1.5 text-left hover:bg-muted/50 transition-colors ${
                  activeId === s.id ? "bg-muted" : ""
                }`}
              >
                {activeId === s.id && (
                  <span className="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-5 bg-primary rounded-r-full" />
                )}
                <span className="text-sm truncate">{s.name}</span>
                <span className="text-[11px] text-muted-foreground truncate">
                  {s.word_count} {t("styleSample.charCount")}
                </span>
              </button>
            ))}
            {loading && (
              <div className="flex justify-center py-3">
                <Loader2 className="w-4 h-4 animate-spin text-muted-foreground" />
              </div>
            )}
          </>
        )}
      </div>
    </>
  );
}

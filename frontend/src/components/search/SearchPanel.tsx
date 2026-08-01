import { useState, useEffect, useRef, useCallback } from "react";
import {
  Search,
  X,
  User,
  MapPin,
  History,
  GitBranch,
  FileText,
  Sparkles,
  Loader2,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { SearchAll } from "@/lib/wailsjs/go/app/App";
import { search } from "@/lib/wailsjs/go/models";
import type { PanelId } from "@/types/panel";

export type SearchResult = search.Result;

interface Props {
  novelId: number;
  query: string;
  results: SearchResult[];
  onResultsChange: (query: string, results: SearchResult[]) => void;
  onNavigateEntity: (panelId: PanelId, entityId: number) => void;
  onNavigateChapter: (
    filePath: string,
    title: string,
    chapterNum: number,
    matchPos: number,
    matchLen: number,
  ) => void;
}

const TYPE_CONFIG: Record<string, { icon: typeof Search; labelKey: string }> = {
  content: { icon: FileText, labelKey: "search.textMatch" },
  character: { icon: User, labelKey: "search.character" },
  location: { icon: MapPin, labelKey: "search.location" },
  timeline: { icon: History, labelKey: "search.timeline" },
  storyarc: { icon: GitBranch, labelKey: "search.storyArc" },
  chapter: { icon: FileText, labelKey: "search.chapter" },
  rag: { icon: Sparkles, labelKey: "search.semanticMatch" },
};

const GROUP_ORDER = [
  "content",
  "character",
  "location",
  "chapter",
  "timeline",
  "storyarc",
  "rag",
];

export default function SearchPanel({
  novelId,
  query,
  results,
  onResultsChange,
  onNavigateEntity,
  onNavigateChapter,
}: Props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [selectedIdx, setSelectedIdx] = useState(-1);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const timerRef = useRef<number>(0);
  const reqIdRef = useRef(0);
  const onResultsChangeRef = useRef(onResultsChange);
  useEffect(() => {
    onResultsChangeRef.current = onResultsChange;
  }, [onResultsChange]);

  const doSearch = useCallback(
    async (q: string, reqId: number) => {
      if (!q.trim() || !novelId) {
        onResultsChangeRef.current(q, []);
        setLoading(false);
        return;
      }
      setLoading(true);
      try {
        const data = (await SearchAll(
          novelId,
          q.trim(),
        )) as unknown as SearchResult[];
        if (reqIdRef.current !== reqId) return;
        setSelectedIdx(-1);
        onResultsChangeRef.current(q, data ?? []);
      } catch {
        if (reqIdRef.current !== reqId) return;
        onResultsChangeRef.current(q, []);
      } finally {
        if (reqIdRef.current === reqId) setLoading(false);
      }
    },
    [novelId],
  );

  useEffect(() => {
    clearTimeout(timerRef.current);
    reqIdRef.current++;
    const id = reqIdRef.current;
    timerRef.current = window.setTimeout(() => doSearch(query, id), 300);
    return () => clearTimeout(timerRef.current);
  }, [query, doSearch]);

  // 按分组整理结果
  const grouped = (() => {
    const map = new Map<string, SearchResult[]>();
    for (const r of results) {
      const existing = map.get(r.type) ?? [];
      existing.push(r);
      map.set(r.type, existing);
    }
    const ordered: {
      type: string;
      label: string;
      icon: typeof Search;
      items: SearchResult[];
    }[] = [];
    for (const gt of GROUP_ORDER) {
      const items = map.get(gt);
      if (items && items.length > 0) {
        ordered.push({
          type: gt,
          label: t(TYPE_CONFIG[gt]?.labelKey ?? gt),
          icon: TYPE_CONFIG[gt]?.icon ?? FileText,
          items,
        });
      }
    }
    return ordered;
  })();

  // 扁平列表用于键盘导航
  const flatList = grouped.flatMap((g) => g.items);

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setSelectedIdx((prev) => Math.min(prev + 1, flatList.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setSelectedIdx((prev) => Math.max(prev - 1, -1));
    } else if (
      e.key === "Enter" &&
      selectedIdx >= 0 &&
      selectedIdx < flatList.length
    ) {
      selectResult(flatList[selectedIdx]);
    } else if (e.key === "Escape") {
      onResultsChange("", []);
      inputRef.current?.blur();
    }
  }

  function selectResult(r: SearchResult) {
    if (r.type === "content" || r.type === "rag" || r.type === "chapter") {
      const displayTitle = r.title
        ? t("search.chapterN", { n: r.chapter_num }) + ` ${r.title}`
        : t("search.chapterN", { n: r.chapter_num });
      onNavigateChapter(
        r.file_path,
        displayTitle,
        r.chapter_num,
        r.match_position ?? 0,
        r.match_len ?? 0,
      );
    } else {
      onNavigateEntity(r.panel_id as PanelId, r.id);
    }
  }

  function clearSearch() {
    onResultsChange("", []);
    inputRef.current?.focus();
  }

  // auto-focus
  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  return (
    <div className="flex flex-col h-full">
      {/* 搜索输入区 */}
      <div className="flex items-center gap-1.5 px-2 py-2 border-b">
        <div className="relative flex-1">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground" />
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={(e) => onResultsChange(e.target.value, [])}
            onKeyDown={handleKeyDown}
            placeholder={t("search.searchPlaceholder")}
            className="w-full h-7 rounded-md border bg-background pl-7 pr-7 text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
          {(query || loading) && (
            <button
              onClick={clearSearch}
              className="absolute right-1.5 top-1/2 -translate-y-1/2 w-4 h-4 flex items-center justify-center text-muted-foreground hover:text-foreground"
            >
              {loading ? (
                <Loader2 className="w-3 h-3 animate-spin" />
              ) : (
                <X className="w-3 h-3" />
              )}
            </button>
          )}
        </div>
      </div>

      {/* 结果区 */}
      <div ref={listRef} className="flex-1 overflow-y-auto overscroll-contain">
        {!query.trim() ? (
          <div className="flex items-center justify-center h-full">
            <p className="text-xs text-muted-foreground">
              {t("search.inputKeyword")}
            </p>
          </div>
        ) : loading ? (
          <div className="flex items-center justify-center h-20">
            <Loader2 className="w-4 h-4 animate-spin text-muted-foreground" />
          </div>
        ) : grouped.length === 0 ? (
          <div className="flex items-center justify-center h-full">
            <p className="text-xs text-muted-foreground">
              {t("search.noResults")}
            </p>
          </div>
        ) : (
          <div className="py-2">
            {grouped.map((group) => {
              const Icon = group.icon;
              return (
                <div key={group.type} className="mb-3">
                  <div className="flex items-center gap-1.5 px-3 py-1">
                    <Icon className="w-3 h-3 text-muted-foreground" />
                    <span className="text-[11px] font-medium text-muted-foreground uppercase tracking-wider">
                      {group.label} ({group.items.length})
                    </span>
                  </div>
                  {group.items.map((r, i) => {
                    const flatIdx = flatList.indexOf(r);
                    const isSelected = flatIdx === selectedIdx;
                    return (
                      <button
                        key={`${r.type}-${r.id || i}-${r.chapter_num}`}
                        onClick={() => selectResult(r)}
                        className={`w-full text-left px-3 py-1.5 hover:bg-muted/50 transition-colors ${
                          isSelected ? "bg-muted" : ""
                        }`}
                      >
                        <div className="flex items-center gap-2">
                          <span className="text-sm truncate flex-1">
                            {r.title}
                          </span>
                          {r.subtitle ? (
                            <span className="text-[10px] text-muted-foreground shrink-0">
                              {r.subtitle}
                            </span>
                          ) : null}
                          {r.relevance > 0 && r.type === "rag" ? (
                            <span className="text-[10px] text-muted-foreground shrink-0">
                              {Math.round(r.relevance * 100)}%
                            </span>
                          ) : null}
                        </div>
                        {r.match_hit ? (
                          <p className="text-[11px] text-muted-foreground leading-relaxed mt-0.5">
                            {r.match_prefix ?? ""}
                            <mark>{r.match_hit}</mark>
                            {r.match_suffix ?? ""}
                          </p>
                        ) : r.match_prefix ? (
                          <p className="text-[11px] text-muted-foreground leading-relaxed mt-0.5">
                            {r.match_prefix}
                          </p>
                        ) : null}
                      </button>
                    );
                  })}
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}

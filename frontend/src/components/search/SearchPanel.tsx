import { useState, useEffect, useRef, useCallback } from "react";
import {
  Search,
  User,
  MapPin,
  History,
  GitBranch,
  FileText,
  Eye,
  Sparkles,
  Loader2,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { SearchAll } from "@/lib/wailsjs/go/app/App";
import { search } from "@/lib/wailsjs/go/models";
import type { PanelId } from "@/types/panel";
import SearchInput from "@/components/shared/SearchInput";

export type SearchResult = search.Result;

interface Props {
  novelId: number;
  query: string;
  results: SearchResult[];
  onResultsChange: (query: string, results: SearchResult[]) => void;
  // 4b: type 透传——storyarc 全局搜索区分 arc/node 跳转（其他领域 undefined）。
  onNavigateEntity: (
    panelId: PanelId,
    entityId: number,
    type?: "arc" | "node",
  ) => void;
  onNavigateChapter: (
    filePath: string,
    title: string,
    chapterNum: number,
    matchPos: number,
    matchLen: number,
  ) => void;
}

const TYPE_CONFIG: Record<string, {
  icon: typeof Search;
  labelKey: string;
  // 4b: 有此字段的领域，subtitle 走 i18n（t(prefix + subtitle)）；无则原样显示。
  subtitlePrefix?: string;
}> = {
  content: { icon: FileText, labelKey: "search.textMatch" },
  character: { icon: User, labelKey: "search.character" },
  location: { icon: MapPin, labelKey: "search.location" },
  timeline: { icon: History, labelKey: "search.timeline", subtitlePrefix: "timeline." },
  storyarc: { icon: GitBranch, labelKey: "search.storyArc", subtitlePrefix: "storyarc." },
  arc_node: { icon: GitBranch, labelKey: "search.arcNode" },
  reader: { icon: Eye, labelKey: "search.reader", subtitlePrefix: "reader." },
  chapter: { icon: FileText, labelKey: "search.chapter", subtitlePrefix: "chapter." },
  rag: { icon: Sparkles, labelKey: "search.semanticMatch" },
};

const GROUP_ORDER = [
  "content",
  "character",
  "location",
  "chapter",
  "timeline",
  "storyarc",
  "arc_node",
  "reader",
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
      // 4b: 全局搜索 storyarc 区分 arc/node——后端 Type "storyarc" → "arc"，"arc_node" → "node"，
      // 其他领域 undefined。focusStore 写入 type，ArcListView 的 useEffect 按 type 走对应分支。
      let focusType: "arc" | "node" | undefined;
      if (r.type === "storyarc") focusType = "arc";
      else if (r.type === "arc_node") focusType = "node";
      onNavigateEntity(r.panel_id as PanelId, r.id, focusType);
    }
  }

  // auto-focus
  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  return (
    <div className="flex flex-col h-full">
      {/* 搜索输入区 */}
      <div className="flex items-center gap-1.5 px-2 py-2 border-b">
        <SearchInput
          ref={inputRef}
          value={query}
          onChange={(v) => onResultsChange(v, [])}
          onKeyDown={handleKeyDown}
          placeholder={t("search.searchPlaceholder")}
          loading={loading}
          className="flex-1"
        />
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
                              {(() => {
                                const cfg = TYPE_CONFIG[r.type];
                                return cfg?.subtitlePrefix
                                  ? t(`${cfg.subtitlePrefix}${r.subtitle}`)
                                  : r.subtitle;
                              })()}
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

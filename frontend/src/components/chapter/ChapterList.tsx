import { useState, useEffect, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { ChevronRight, FileText, Pencil, Plus, Download } from "lucide-react";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";
import { Button } from "@/components/ui/button";
import type { chapter } from "@/lib/wailsjs/go/models";
import { EventsOn } from "@/lib/wailsjs/runtime/runtime";
import { chapterKeys, maxChapterKeys } from "@/lib/queryKeys";
import { useChapters } from "./useChapters";
import { useCreateChapter } from "./useCreateChapter";
import { useUpdateChapterTitle } from "./useUpdateChapterTitle";

interface Props {
  novelId: number;
  target: { path: string; title: string } | null;
  onSelectChapter: (ch: chapter.Chapter) => void;
  onSelectGoink: () => void;
  onExportNovel: () => void;
}

const BLOCK_SIZE = 100;

export default function ChapterList({
  novelId,
  target,
  onSelectChapter,
  onSelectGoink,
  onExportNovel,
}: Props) {
  const { t } = useTranslation();
  const qc = useQueryClient();

  // 5.2 commit 1: GetChapters 走 query（直接 import wailsjs，不用 useApp）。
  // query 错误走全局中间件，组件加 isError 内连显示 + retry（对齐 ReaderList/PreferenceList）。
  // 5.2 commit 2: CreateChapter/UpdateChapterTitle 走 mutation（onSuccess invalidate 接管 refetch）。
  const { data: chapters = [], isError, refetch } = useChapters(novelId);
  const createChapterMutation = useCreateChapter(novelId);
  const updateChapterTitleMutation = useUpdateChapterTitle(novelId);
  const [chapterTitle, setChapterTitle] = useState("");
  const [showCreateChapter, setShowCreateChapter] = useState(false);
  const [expandedBlocks, setExpandedBlocks] = useState<Set<number>>(new Set());
  const [editingId, setEditingId] = useState<number | null>(null);
  const [editTitle, setEditTitle] = useState("");
  const [createError, setCreateError] = useState("");

  // file:changed 时刷新章节列表（字数统计、新章等）
  // 5.2 commit 3: 改 qc.invalidateQueries（chapterKeys.list 活跃 query 自动 refetch）。
  // path 属 chapters/ 时同时失效 maxChapterKeys（storyarc windowCenter 跟随，补 AI 写章场景）。
  // refetch 保留给 isError retry 按钮（立即重拉语义，与 invalidate 标 stale 不同用途）。
  useEffect(() => {
    const unsub = EventsOn("file:changed", (data: any) => {
      if (data.novel_id !== novelId) return;
      if (
        data.path &&
        (data.path.startsWith("chapters/") ||
          data.path.startsWith("outlines/") ||
          data.path === "goink.md")
      ) {
        qc.invalidateQueries({ queryKey: chapterKeys.list(novelId) });
        if (data.path.startsWith("chapters/")) {
          qc.invalidateQueries({ queryKey: maxChapterKeys.detail(novelId) });
        }
      }
    });
    return () => unsub();
  }, [novelId, qc]);

  // ── 章节分块 ────────────────────────────────────────────

  const chapterBlocks = useMemo(() => {
    const sorted = [...chapters].sort(
      (a, b) => b.chapter_number - a.chapter_number,
    );
    const blocks: {
      key: number;
      start: number;
      end: number;
      chs: chapter.Chapter[];
    }[] = [];
    for (let i = 0; i < sorted.length; i += BLOCK_SIZE) {
      const slice = sorted.slice(i, Math.min(i + BLOCK_SIZE, sorted.length));
      slice.sort((a, b) => a.chapter_number - b.chapter_number);
      blocks.push({
        key: i / BLOCK_SIZE,
        start: slice[0].chapter_number,
        end: slice[slice.length - 1].chapter_number,
        chs: slice,
      });
    }
    return blocks;
  }, [chapters]);

  function toggleBlock(key: number) {
    setExpandedBlocks((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }

  async function handleCreateChapter() {
    if (!chapterTitle.trim()) return;
    try {
      await createChapterMutation.mutateAsync({
        novel_id: novelId,
        title: chapterTitle.trim(),
      });
      setChapterTitle("");
      setShowCreateChapter(false);
      setCreateError("");
      // 5.2 commit 2: onSuccess invalidate chapterKeys.list + maxChapterKeys，不需要 refetch
    } catch (err) {
      setCreateError(toErrorMessage(err));
    }
  }

  function startEdit(ch: chapter.Chapter) {
    setEditingId(ch.id);
    setEditTitle(ch.title);
  }

  async function commitEdit() {
    if (editingId == null) return;
    const ch = chapters.find((c) => c.id === editingId);
    if (!ch) return;
    const newTitle = editTitle.trim();
    if (newTitle && newTitle !== ch.title) {
      try {
        await updateChapterTitleMutation.mutateAsync({
          chapterNumber: ch.chapter_number,
          title: newTitle,
        });
        // 5.2 commit 2: onSuccess invalidate chapterKeys.list，不需要 refetch
      } catch (err) {
        toastError(t("common.saveFailed") + ": " + toErrorMessage(err));
        console.error(err);
      }
    }
    setEditingId(null);
  }

  function cancelEdit() {
    setEditingId(null);
  }

  return (
    <>
      <div className="flex items-center justify-between px-3 py-2.5 border-b">
        <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          {t("sidebar.chaptersCount", { count: chapters.length })}
        </span>
        <div className="flex items-center gap-0.5">
          <button
            onClick={onExportNovel}
            className="w-6 h-6 flex items-center justify-center rounded hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
            title={t("sidebar.export")}
          >
            <Download className="w-3.5 h-3.5" />
          </button>
          <button
            onClick={() => setShowCreateChapter(true)}
            className="w-6 h-6 flex items-center justify-center rounded hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
          >
            <Plus className="w-4 h-4" />
          </button>
        </div>
      </div>

      {showCreateChapter && (
        <div className="p-3 border-b space-y-2">
          <input
            type="text"
            value={chapterTitle}
            autoFocus
            onChange={(e) => {
              setChapterTitle(e.target.value);
              setCreateError("");
            }}
            onKeyDown={(e) => e.key === "Enter" && handleCreateChapter()}
            placeholder={t("sidebar.chapterTitle")}
            className="w-full h-8 rounded-md border bg-background px-2.5 text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
          {createError && (
            <p className="text-xs text-destructive">{createError}</p>
          )}
          <div className="flex gap-2">
            <Button size="sm" onClick={handleCreateChapter}>
              {t("sidebar.add")}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => {
                setShowCreateChapter(false);
                setChapterTitle("");
                setCreateError("");
              }}
            >
              {t("sidebar.cancel")}
            </Button>
          </div>
        </div>
      )}

      <button
        onClick={onSelectGoink}
        className={`w-full flex items-center gap-2.5 px-3 py-1.5 text-left hover:bg-muted/50 transition-colors relative border-b border-border/50
          ${target?.path === "goink.md" ? "bg-primary/10 font-medium" : ""}`}
      >
        {target?.path === "goink.md" && (
          <span className="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-5 bg-primary rounded-r-full" />
        )}
        <FileText className="w-3.5 h-3.5 text-muted-foreground shrink-0" />
        <span className="flex-1 text-sm truncate">
          {t("sidebar.storyStatus")}
        </span>
      </button>

      <div className="flex-1 overflow-y-auto overscroll-contain">
        {isError ? (
          <div className="flex items-center justify-center h-full">
            <div className="text-center">
              <FileText className="w-8 h-8 text-muted-foreground/30 mx-auto mb-2" />
              <p className="text-xs text-destructive">{t("chapter.loadFailed")}</p>
              <button
                onClick={() => refetch()}
                className="text-xs text-primary underline mt-1"
              >
                {t("common.retry")}
              </button>
            </div>
          </div>
        ) : chapters.length === 0 ? (
          <div className="flex items-center justify-center h-full">
            <div className="text-center">
              <FileText className="w-8 h-8 text-muted-foreground/30 mx-auto mb-2" />
              <p className="text-xs text-muted-foreground">
                {t("sidebar.noChapters")}
              </p>
              <p className="text-xs text-muted-foreground/60 mt-0.5">
                {t("sidebar.createFirstChapter")}
              </p>
            </div>
          </div>
        ) : (
          chapterBlocks.map((block) => {
            const isExpanded = expandedBlocks.has(block.key);
            const range =
              block.start === block.end
                ? t("sidebar.chapterN", { n: block.start })
                : t("sidebar.chapterRange", {
                    start: block.start,
                    end: block.end,
                  });
            return (
              <div key={block.key}>
                <button
                  onClick={() => toggleBlock(block.key)}
                  className="w-full flex items-center gap-1.5 px-3 py-1.5 text-left hover:bg-muted/30 transition-colors border-b border-border/50"
                >
                  <ChevronRight
                    className={`w-3.5 h-3.5 text-muted-foreground shrink-0 transition-transform duration-200 ${isExpanded ? "rotate-90" : ""}`}
                  />
                  <span className="text-xs text-muted-foreground">{range}</span>
                  <span className="text-[10px] text-muted-foreground/50 ml-auto">
                    {t("sidebar.chapterCountShort", {
                      count: block.chs.length,
                    })}
                  </span>
                </button>
                {isExpanded && (
                  <div>
                    {block.chs.map((ch) => (
                      <div
                        key={ch.id}
                        className="group flex items-center w-full relative"
                      >
                        <button
                          onClick={() => onSelectChapter(ch)}
                          className={`flex items-center gap-2.5 pl-5 pr-2 py-1.5 text-left hover:bg-muted/50 transition-colors flex-1 min-w-0
                            ${target?.path === ch.file_path ? "bg-primary/10 font-medium" : ""}`}
                        >
                          {target?.path === ch.file_path && (
                            <span className="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-5 bg-primary rounded-r-full" />
                          )}
                          <span className="text-xs text-muted-foreground shrink-0 whitespace-nowrap tabular-nums">
                            {t("sidebar.chapterN", { n: ch.chapter_number })}
                          </span>
                          {editingId === ch.id ? (
                            <input
                              value={editTitle}
                              onChange={(e) => setEditTitle(e.target.value)}
                              onKeyDown={(e) => {
                                if (e.key === "Enter") commitEdit();
                                if (e.key === "Escape") cancelEdit();
                              }}
                              onBlur={commitEdit}
                              autoFocus
                              onClick={(e) => e.stopPropagation()}
                              className="flex-1 h-6 rounded border bg-background px-1.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                            />
                          ) : (
                            <span
                              className="flex-1 text-sm truncate"
                              title={ch.title}
                            >
                              {ch.title}
                            </span>
                          )}
                          {ch.word_count > 0 && editingId !== ch.id && (
                            <span className="text-[10px] text-muted-foreground/60 shrink-0">
                              {t("sidebar.wordCount", { count: ch.word_count })}
                            </span>
                          )}
                        </button>
                        {editingId !== ch.id && (
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              startEdit(ch);
                            }}
                            className="absolute right-0 top-1/2 -translate-y-1/2 w-6 h-6 flex items-center justify-center rounded opacity-0 group-hover:opacity-100 hover:bg-muted text-muted-foreground hover:text-foreground transition-all z-10"
                          >
                            <Pencil className="w-3 h-3" />
                          </button>
                        )}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            );
          })
        )}
      </div>
    </>
  );
}

import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronRight, Pencil, Plus } from "lucide-react";
import type { chapter } from "@/lib/wailsjs/go/models";
import PopSelect from "@/components/shared/PopSelect";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";

export const SIDEBAR_BLOCK_SIZE = 100;

interface Props {
  name: string;
  chapters: chapter.Chapter[];
  expanded: boolean;
  selectedPath?: string;
  rangeIndex: number;
  busy: boolean;
  onToggle: () => void;
  onRangeSelect: (index: number) => void;
  onCreate: () => void;
  onSelectChapter: (item: chapter.Chapter) => void;
  onRenameChapter: (item: chapter.Chapter, title: string) => Promise<void>;
}

export default function SidebarChapterGroup({
  name,
  chapters,
  expanded,
  selectedPath,
  rangeIndex,
  busy,
  onToggle,
  onRangeSelect,
  onCreate,
  onSelectChapter,
  onRenameChapter,
}: Props) {
  const { t } = useTranslation();
  const [editingId, setEditingId] = useState<number | null>(null);
  const [editTitle, setEditTitle] = useState("");
  const committingRef = useRef(false);
  const ignoreBlurRef = useRef(false);
  const blocks: chapter.Chapter[][] = [];
  if (chapters.length > SIDEBAR_BLOCK_SIZE) {
    for (let start = 0; start < chapters.length; start += SIDEBAR_BLOCK_SIZE) {
      blocks.push(chapters.slice(start, start + SIDEBAR_BLOCK_SIZE));
    }
  }
  const visibleChapters =
    blocks.length === 0
      ? chapters
      : blocks[Math.min(rangeIndex, blocks.length - 1)];

  async function commitEdit(item: chapter.Chapter) {
    if (ignoreBlurRef.current || committingRef.current || editingId !== item.id)
      return;
    ignoreBlurRef.current = true;
    committingRef.current = true;
    const title = editTitle.trim();
    setEditingId(null);
    try {
      if (title && title !== item.title) await onRenameChapter(item, title);
    } catch (error) {
      toastError(t("common.saveFailed") + ": " + toErrorMessage(error));
    } finally {
      committingRef.current = false;
    }
  }

  return (
    <section aria-label={name} className="border-b border-border/50">
      <div className="group flex items-center hover:bg-muted/30">
        <button
          type="button"
          onClick={onToggle}
          aria-expanded={expanded}
          className="flex min-w-0 flex-1 items-center gap-1.5 px-3 py-2 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
        >
          <ChevronRight
            aria-hidden="true"
            className={`h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform ${expanded ? "rotate-90" : ""}`}
          />
          <span
            className="min-w-0 flex-1 truncate text-sm font-medium"
            title={name}
          >
            {name}
          </span>
          <span className="shrink-0 text-[10px] text-muted-foreground">
            {t("sidebar.chapterCountShort", { count: chapters.length })}
          </span>
        </button>
        <button
          type="button"
          onClick={onCreate}
          disabled={busy}
          aria-label={t("chapterManagement.addToGroup", { name })}
          title={t("chapterManagement.addToGroup", { name })}
          className="mr-2 rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
        >
          <Plus aria-hidden="true" className="h-3.5 w-3.5" />
        </button>
      </div>
      {expanded && (
        <div>
          {blocks.length > 0 && (
            <div className="px-5 pb-2">
              <PopSelect
                value={String(Math.min(rangeIndex, blocks.length - 1))}
                options={blocks.map((block, index) => ({
                  value: String(index),
                  label: t("sidebar.chapterRange", {
                    start: block[0].reading_number,
                    end: block[block.length - 1].reading_number,
                  }),
                }))}
                onChange={(value) => onRangeSelect(Number(value))}
                ariaLabel={t("sidebar.chapterSection", { name })}
                className="w-full"
                minWidth="0"
                dropUp={false}
              />
            </div>
          )}
          {visibleChapters.length === 0 ? (
            <p className="px-5 pb-3 text-xs text-muted-foreground">
              {t("sidebar.noChapters")}
            </p>
          ) : (
            <ol>
              {visibleChapters.map((item) => {
                const selected =
                  selectedPath === item.file_path ||
                  selectedPath === item.outline_file_path;
                return (
                  <li
                    key={item.id}
                    className="group relative flex min-w-0 items-center"
                  >
                    {editingId === item.id ? (
                      <input
                        value={editTitle}
                        onChange={(event) => setEditTitle(event.target.value)}
                        onKeyDown={(event) => {
                          if (event.key === "Enter") {
                            event.preventDefault();
                            void commitEdit(item);
                          }
                          if (event.key === "Escape") {
                            ignoreBlurRef.current = true;
                            setEditingId(null);
                          }
                        }}
                        onBlur={() => void commitEdit(item)}
                        autoFocus
                        aria-label={t("sidebar.chapterTitle")}
                        className="mx-5 my-1 h-7 min-w-0 flex-1 rounded border bg-background px-1.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                      />
                    ) : (
                      <button
                        type="button"
                        onClick={() => onSelectChapter(item)}
                        className={`flex min-w-0 flex-1 items-center gap-2 px-5 py-1.5 text-left hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring ${selected ? "bg-primary/10 font-medium" : ""}`}
                      >
                        {selected && (
                          <span className="absolute left-0 top-1/2 h-5 w-0.5 -translate-y-1/2 rounded-r-full bg-primary" />
                        )}
                        <span className="shrink-0 whitespace-nowrap text-xs tabular-nums text-muted-foreground">
                          {t("sidebar.chapterN", { n: item.reading_number })}
                        </span>
                        <span
                          className="min-w-0 flex-1 truncate text-sm"
                          title={item.title}
                        >
                          {item.title}
                        </span>
                        {item.word_count > 0 && (
                          <span className="shrink-0 text-[10px] text-muted-foreground/60">
                            {t("sidebar.wordCount", { count: item.word_count })}
                          </span>
                        )}
                      </button>
                    )}
                    {editingId !== item.id && (
                      <button
                        type="button"
                        onClick={() => {
                          ignoreBlurRef.current = false;
                          setEditingId(item.id);
                          setEditTitle(item.title);
                        }}
                        aria-label={t("sidebar.renameChapter", {
                          title: item.title,
                        })}
                        className="absolute right-0 top-1/2 z-10 flex h-6 w-6 -translate-y-1/2 items-center justify-center rounded text-muted-foreground opacity-0 transition-opacity hover:bg-muted hover:text-foreground focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring group-hover:opacity-100"
                      >
                        <Pencil aria-hidden="true" className="h-3 w-3" />
                      </button>
                    )}
                  </li>
                );
              })}
            </ol>
          )}
        </div>
      )}
    </section>
  );
}

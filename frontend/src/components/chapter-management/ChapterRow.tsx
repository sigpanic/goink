import { useEffect, useRef, type DragEvent } from "react";
import { useTranslation } from "react-i18next";
import { Check, GripVertical, MoreHorizontal } from "lucide-react";
import type { chapter } from "@/lib/wailsjs/go/models";
import "./ChapterManagementDrag.css";

export type ChapterDropEdge = "before" | "after";

interface Props {
  item: chapter.Chapter;
  busy: boolean;
  menuOpen: boolean;
  dragChapterId: number | null;
  dropEdge: ChapterDropEdge | null;
  highlighted: boolean;
  onToggleMenu: () => void;
  onCloseMenu: () => void;
  onInsertBefore: () => void;
  onInsertAfter: () => void;
  onMove: () => void;
  onDelete: () => void;
  onDragStart: () => void;
  onDragEnd: () => void;
  onDragOver: (edge: ChapterDropEdge) => void;
  onDragLeave: () => void;
  onDrop: (edge: ChapterDropEdge) => void;
}

export default function ChapterRow({
  item,
  busy,
  menuOpen,
  dragChapterId,
  dropEdge,
  highlighted,
  onToggleMenu,
  onCloseMenu,
  onInsertBefore,
  onInsertAfter,
  onMove,
  onDelete,
  onDragStart,
  onDragEnd,
  onDragOver,
  onDragLeave,
  onDrop,
}: Props) {
  const { t } = useTranslation();
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!menuOpen) return;

    function handlePointerDown(event: PointerEvent) {
      if (!menuRef.current?.contains(event.target as Node)) onCloseMenu();
    }

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") onCloseMenu();
    }

    document.addEventListener("pointerdown", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("pointerdown", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [menuOpen, onCloseMenu]);

  function handleDragStart(event: DragEvent<HTMLButtonElement>) {
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.setData(
      "application/x-goink-chapter-id",
      String(item.id),
    );
    onDragStart();
  }

  function handleDragOver(event: DragEvent<HTMLLIElement>) {
    if (dragChapterId === null || busy) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = "move";
    onDragOver(dropEdgeFor(event));
  }

  function handleDrop(event: DragEvent<HTMLLIElement>) {
    if (dragChapterId === null) return;
    event.preventDefault();
    onDrop(dropEdgeFor(event));
  }

  function dropEdgeFor(event: DragEvent<HTMLLIElement>): ChapterDropEdge {
    const rect = event.currentTarget.getBoundingClientRect();
    if (rect.height <= 0) return "before";
    return event.clientY < rect.top + rect.height / 2 ? "before" : "after";
  }

  function handleDragLeave(event: DragEvent<HTMLLIElement>) {
    if (event.currentTarget.contains(event.relatedTarget as Node | null))
      return;
    onDragLeave();
  }

  return (
    <li
      data-chapter-id={item.id}
      onDragOver={handleDragOver}
      onDrop={handleDrop}
      onDragLeave={handleDragLeave}
      className={`chapter-row flex items-center gap-3 px-4 py-3 ${dragChapterId === item.id ? "chapter-row-dragging" : ""} ${dropEdge ? `chapter-row-drop-${dropEdge}` : ""} ${highlighted ? "chapter-row-moved" : ""}`}
    >
      <button
        type="button"
        draggable={!busy}
        onDragStart={handleDragStart}
        onDragEnd={onDragEnd}
        aria-label={t("chapterManagement.dragChapter", { title: item.title })}
        className="cursor-grab text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <GripVertical aria-hidden="true" className="h-4 w-4" />
      </button>
      <span className="w-16 shrink-0 text-sm tabular-nums text-muted-foreground">
        {t("sidebar.chapterN", { n: item.reading_number })}
      </span>
      <span className="min-w-0 flex-1 truncate text-sm" title={item.title}>
        {item.title}
      </span>
      {highlighted && (
        <span
          role="status"
          className="chapter-row-moved-badge shrink-0 text-xs font-medium"
        >
          <Check aria-hidden="true" className="h-3 w-3" />
          {t("chapterManagement.moveSuccess")}
        </span>
      )}
      <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
        {t("sidebar.wordCount", { count: item.word_count })}
      </span>
      <div ref={menuRef} className="relative shrink-0">
        <button
          type="button"
          aria-label={t("chapterManagement.chapterActions", {
            title: item.title,
          })}
          aria-expanded={menuOpen}
          onClick={onToggleMenu}
          className="rounded-md p-1.5 text-muted-foreground hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <MoreHorizontal aria-hidden="true" className="h-4 w-4" />
        </button>
        {menuOpen && (
          <div className="absolute right-0 top-full z-20 mt-1 min-w-40 rounded-md border bg-popover p-1 shadow-lg">
            <button
              type="button"
              onClick={onInsertBefore}
              className="w-full rounded px-2 py-1.5 text-left text-sm hover:bg-muted"
            >
              {t("chapterManagement.insertBeforeChapter")}
            </button>
            <button
              type="button"
              onClick={onInsertAfter}
              className="w-full rounded px-2 py-1.5 text-left text-sm hover:bg-muted"
            >
              {t("chapterManagement.insertAfterChapter")}
            </button>
            <button
              type="button"
              onClick={onMove}
              className="w-full rounded px-2 py-1.5 text-left text-sm hover:bg-muted"
            >
              {t("chapterManagement.moveChapter")}
            </button>
            <button
              type="button"
              onClick={onDelete}
              className="w-full rounded px-2 py-1.5 text-left text-sm text-destructive hover:bg-muted"
            >
              {t("common.delete")}
            </button>
          </div>
        )}
      </div>
    </li>
  );
}

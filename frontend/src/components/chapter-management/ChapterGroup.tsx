import { useState, type DragEvent } from "react";
import { useTranslation } from "react-i18next";
import { ChevronRight, Plus } from "lucide-react";
import type { chapter } from "@/lib/wailsjs/go/models";
import ChapterRow, { type ChapterDropEdge } from "./ChapterRow";

const BLOCK_SIZE = 100;

interface Props {
  groupKey: string;
  name: string;
  items: chapter.Chapter[];
  expanded: boolean;
  busy: boolean;
  menuId: number | null;
  dragChapterId: number | null;
  highlightedChapterId: number | null;
  revealedChapterId: number | null;
  sectionRef: (element: HTMLElement | null) => void;
  onToggle: () => void;
  onCreate: (beforeId: number | null) => void;
  onMove: (item: chapter.Chapter) => void;
  onDelete: (item: chapter.Chapter) => void;
  onMenuChange: (id: number | null) => void;
  onDragStart: (id: number) => void;
  onDragEnd: () => void;
  onDrop: (beforeId: number | null) => void;
  onDismissReveal: () => void;
}

export default function ChapterGroup({
  groupKey,
  name,
  items,
  expanded,
  busy,
  menuId,
  dragChapterId,
  highlightedChapterId,
  revealedChapterId,
  sectionRef,
  onToggle,
  onCreate,
  onMove,
  onDelete,
  onMenuChange,
  onDragStart,
  onDragEnd,
  onDrop,
  onDismissReveal,
}: Props) {
  const { t } = useTranslation();
  const [openRanges, setOpenRanges] = useState<Set<string>>(new Set());
  const [dropTarget, setDropTarget] = useState<
    "group" | { id: number; edge: ChapterDropEdge } | null
  >(null);
  const nextIdById = new Map(
    items.map((item, index) => [item.id, items[index + 1]?.id ?? null]),
  );
  const blocks: chapter.Chapter[][] = [];
  if (expanded && items.length > BLOCK_SIZE) {
    for (let start = 0; start < items.length; start += BLOCK_SIZE) {
      blocks.push(items.slice(start, start + BLOCK_SIZE));
    }
  }

  function toggleRange(key: string, revealed: boolean) {
    if (revealed) onDismissReveal();
    setOpenRanges((previous) => {
      const next = new Set(previous);
      if (next.has(key)) next.delete(key);
      else if (!revealed) next.add(key);
      return next;
    });
  }

  function dragOver(event: DragEvent<HTMLElement>) {
    if (dragChapterId === null || busy) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = "move";
    setDropTarget("group");
  }

  function drop(event: DragEvent<HTMLElement>, beforeId: number | null) {
    if (dragChapterId === null) return;
    event.preventDefault();
    setDropTarget(null);
    onDrop(beforeId);
  }

  function renderRows(rows: chapter.Chapter[]) {
    if (rows.length === 0) {
      return (
        <p className="px-4 py-5 text-sm text-muted-foreground">
          {t("chapterManagement.emptyGroup")}
        </p>
      );
    }
    return (
      <ol className="divide-y divide-border">
        {rows.map((item) => (
          <ChapterRow
            key={item.id}
            item={item}
            busy={busy}
            menuOpen={menuId === item.id}
            dragChapterId={dragChapterId}
            dropEdge={
              dropTarget !== null &&
              dropTarget !== "group" &&
              dropTarget.id === item.id
                ? dropTarget.edge
                : null
            }
            highlighted={highlightedChapterId === item.id}
            onToggleMenu={() =>
              onMenuChange(menuId === item.id ? null : item.id)
            }
            onCloseMenu={() => onMenuChange(null)}
            onInsertBefore={() => {
              onMenuChange(null);
              onCreate(item.id);
            }}
            onInsertAfter={() => {
              onMenuChange(null);
              onCreate(nextIdById.get(item.id) ?? null);
            }}
            onMove={() => {
              onMenuChange(null);
              onMove(item);
            }}
            onDelete={() => {
              onMenuChange(null);
              onDelete(item);
            }}
            onDragStart={() => onDragStart(item.id)}
            onDragEnd={() => {
              setDropTarget(null);
              onDragEnd();
            }}
            onDragOver={(edge) => setDropTarget({ id: item.id, edge })}
            onDragLeave={() =>
              setDropTarget((previous) =>
                previous !== null &&
                previous !== "group" &&
                previous.id === item.id
                  ? null
                  : previous,
              )
            }
            onDrop={(edge) => {
              setDropTarget(null);
              onDrop(
                edge === "before" ? item.id : (nextIdById.get(item.id) ?? null),
              );
            }}
          />
        ))}
      </ol>
    );
  }

  return (
    <section ref={sectionRef} className="scroll-mt-28 rounded-xl border">
      <h2 className="flex items-stretch border-b bg-muted/30">
        <button
          type="button"
          aria-expanded={expanded}
          onClick={onToggle}
          onDragOver={dragOver}
          onDrop={(event) => drop(event, null)}
          onDragLeave={() => setDropTarget(null)}
          className={`flex min-w-0 flex-1 items-center gap-2 px-4 py-3 text-left text-sm font-medium hover:bg-muted/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring ${dropTarget === "group" ? "bg-primary/10 ring-2 ring-inset ring-primary" : ""}`}
        >
          <ChevronRight
            aria-hidden="true"
            className={`h-4 w-4 shrink-0 transition-transform ${expanded ? "rotate-90" : ""}`}
          />
          <span className="min-w-0 flex-1 truncate">{name}</span>
          <span className="text-xs font-normal text-muted-foreground">
            {t("sidebar.chapterCountShort", { count: items.length })}
          </span>
        </button>
        <button
          type="button"
          onClick={() => onCreate(null)}
          aria-label={t("chapterManagement.addToGroup", { name })}
          className="px-4 text-muted-foreground hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
        >
          <Plus aria-hidden="true" className="h-4 w-4" />
        </button>
      </h2>
      {expanded &&
        (blocks.length === 0 ? (
          renderRows(items)
        ) : (
          <div className="divide-y divide-border">
            {blocks.map((block) => {
              const rangeKey = `${groupKey}:${block[0].id}`;
              const revealed = block.some(
                (item) => item.id === revealedChapterId,
              );
              const rangeOpen = openRanges.has(rangeKey) || revealed;
              return (
                <div key={rangeKey}>
                  <button
                    type="button"
                    aria-expanded={rangeOpen}
                    onClick={() => toggleRange(rangeKey, revealed)}
                    className="flex w-full items-center gap-2 px-5 py-3 text-left text-sm hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
                  >
                    <ChevronRight
                      aria-hidden="true"
                      className={`h-4 w-4 shrink-0 transition-transform ${rangeOpen ? "rotate-90" : ""}`}
                    />
                    {t("sidebar.chapterRange", {
                      start: block[0].reading_number,
                      end: block[block.length - 1].reading_number,
                    })}
                  </button>
                  {rangeOpen && renderRows(block)}
                </div>
              );
            })}
          </div>
        ))}
    </section>
  );
}

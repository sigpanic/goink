import { Plus } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import BookCover from "@/components/sidebar/BookCover";
import type { novel } from "@/hooks/useApp";

interface Props {
  novels: novel.Novel[];
  novelId: number;
  onSelectNovel: (n: novel.Novel) => void;
  showCreate: boolean;
  setShowCreate: (v: boolean) => void;
  title: string;
  setTitle: (v: string) => void;
  description: string;
  setDescription: (v: string) => void;
  onCreateNovel: () => void;
}

export default function NovelList({
  novels,
  novelId,
  onSelectNovel,
  showCreate,
  setShowCreate,
  title,
  setTitle,
  description,
  setDescription,
  onCreateNovel,
}: Props) {
  const { t } = useTranslation();
  return (
    <>
      <div className="flex items-center justify-between px-3 py-2.5 border-b">
        <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          {t("sidebar.novelsCount", { count: novels.length })}
        </span>
        <button
          onClick={() => setShowCreate(true)}
          className="w-6 h-6 flex items-center justify-center rounded hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
        >
          <Plus className="w-4 h-4" />
        </button>
      </div>

      {showCreate && (
        <div className="p-3 border-b space-y-2">
          <input
            type="text"
            value={title}
            autoFocus
            onChange={(e) => setTitle(e.target.value)}
            placeholder={t("sidebar.bookTitle")}
            className="w-full h-8 rounded-md border bg-background px-2.5 text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
          <input
            type="text"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder={t("sidebar.bookSummary")}
            className="w-full h-8 rounded-md border bg-background px-2.5 text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
          <div className="flex gap-2">
            <Button size="sm" onClick={onCreateNovel}>
              {t("sidebar.create")}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => {
                setShowCreate(false);
                setTitle("");
                setDescription("");
              }}
            >
              {t("sidebar.cancel")}
            </Button>
          </div>
        </div>
      )}

      <div className="flex-1 overflow-y-auto overscroll-contain">
        {novels.map((n) => (
          <button
            key={n.id}
            onClick={() => onSelectNovel(n)}
            className={`w-full flex items-center gap-3 px-3 py-2.5 text-left hover:bg-muted/50 transition-colors relative
              ${n.id === novelId ? "bg-primary/10 text-foreground" : ""}`}
          >
            {n.id === novelId && (
              <span className="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-5 bg-primary rounded-r-full" />
            )}
            <div className="w-8 shrink-0 rounded-sm overflow-hidden">
              <BookCover novelId={n.id} />
            </div>
            <span
              className={`flex-1 text-sm truncate ${n.id === novelId ? "font-medium" : ""}`}
            >
              {n.title}
            </span>
          </button>
        ))}
      </div>
    </>
  );
}

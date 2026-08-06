import { useMemo, useState } from "react";
import { Search, Target, Lightbulb } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useTimelineEntries } from "./useTimelineEntries";

interface Props {
  novelId: number;
}

export default function SidebarTimelineList({ novelId }: Props) {
  const { t } = useTranslation();
  const { data: entries = [], isError } = useTimelineEntries(novelId);
  const [search, setSearch] = useState("");

  const filtered = useMemo(() => {
    if (!search.trim()) return entries;
    const q = search.toLowerCase();
    return entries.filter((e) => e.title.toLowerCase().includes(q));
  }, [entries, search]);

  const catIcon = (cat: string) => {
    switch (cat) {
      case "foreshadowing":
        return (
          <Target className="h-3 w-3 text-tag-amber-foreground shrink-0" />
        );
      case "user_directive":
        return (
          <Lightbulb className="h-3 w-3 text-tag-purple-foreground shrink-0" />
        );
      default:
        return <Target className="h-3 w-3 text-muted-foreground shrink-0" />;
    }
  };

  const statusDot = (status: string) => {
    switch (status) {
      case "pending":
        return "bg-tag-blue";
      case "resolved":
        return "bg-tag-green";
      default:
        return "bg-muted";
    }
  };

  return (
    <>
      <div className="flex items-center justify-between px-3 py-2.5 border-b">
        <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          {t("timeline.foreshadowingOrInstruction")} ({entries.length})
        </span>
      </div>
      <div className="px-2 py-1.5 border-b">
        <div className="relative">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-3 h-3 text-muted-foreground" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t("timeline.searchTimeline")}
            className="w-full h-7 rounded-md border bg-background pl-7 pr-2 text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>
      </div>
      <div className="flex-1 overflow-y-auto overscroll-contain">
        {isError ? (
          <div className="flex items-center justify-center h-full">
            <p className="text-xs text-destructive">
              {t("timeline.loadFailed")}
            </p>
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex items-center justify-center h-full">
            <p className="text-xs text-muted-foreground">
              {search
                ? t("timeline.noMatchingEntries")
                : t("timeline.noEntries")}
            </p>
          </div>
        ) : (
          filtered.map((e) => (
            <div
              key={e.id}
              className="w-full flex items-center gap-2 px-3 py-1.5 text-left cursor-pointer hover:bg-muted transition-colors"
            >
              {catIcon(e.category)}
              <div className="flex-1 min-w-0">
                <span className="text-xs truncate block text-foreground">
                  {e.title}
                </span>
                <span className="text-[10px] text-muted-foreground">
                  {t("timeline.targetChapterN2", { n: e.target_chapter })}
                </span>
              </div>
              <span
                className={`shrink-0 h-1.5 w-1.5 rounded-full ${statusDot(e.status)}`}
              />
            </div>
          ))
        )}
      </div>
    </>
  );
}

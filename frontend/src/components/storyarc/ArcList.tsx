import { useState, useEffect, useCallback, useMemo } from "react";
import { Search, GitBranch } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useApp } from "@/hooks/useApp";
import { useRefresh } from "@/hooks/useRefresh";
import type { storyarc } from "@/hooks/useApp";

interface Props {
  novelId: number;
}

export default function SidebarArcList({ novelId }: Props) {
  const app = useApp();
  const { t } = useTranslation();
  const { refreshNonce } = useRefresh();

  const [arcs, setArcs] = useState<storyarc.StoryArc[]>([]);
  const [search, setSearch] = useState("");

  const load = useCallback(async () => {
    if (!novelId) {
      setArcs([]);
      return;
    }
    const list = await app.GetStoryArcs(novelId);
    setArcs(list ?? []);
  }, [novelId, app]);

  useEffect(() => {
    load();
  }, [load, refreshNonce]);

  const filtered = useMemo(() => {
    if (!search.trim()) return arcs;
    const q = search.toLowerCase();
    return arcs.filter((a) => a.name.toLowerCase().includes(q));
  }, [arcs, search]);

  const statusDot = (status: string) => {
    switch (status) {
      case "active":
        return "bg-tag-green";
      case "paused":
        return "bg-tag-amber";
      case "completed":
        return "bg-tag-blue";
      default:
        return "bg-muted";
    }
  };

  return (
    <>
      <div className="flex items-center justify-between px-3 py-2.5 border-b">
        <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          {t("storyarc.narrativeArcs")} ({arcs.length})
        </span>
      </div>
      <div className="px-2 py-1.5 border-b">
        <div className="relative">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-3 h-3 text-muted-foreground" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t("storyarc.searchArcs")}
            className="w-full h-7 rounded-md border bg-background pl-7 pr-2 text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>
      </div>
      <div className="flex-1 overflow-y-auto overscroll-contain">
        {filtered.length === 0 ? (
          <div className="flex items-center justify-center h-full">
            <p className="text-xs text-muted-foreground">
              {search ? t("storyarc.noMatchingArcs") : t("storyarc.noArcs")}
            </p>
          </div>
        ) : (
          filtered.map((a) => (
            <div
              key={a.id}
              className="w-full flex items-center gap-2 px-3 py-1.5 text-left hover:bg-muted/50 transition-colors"
            >
              <span className="shrink-0 flex h-5 w-5 items-center justify-center rounded bg-tag-purple text-tag-purple-foreground">
                <GitBranch className="h-3 w-3" />
              </span>
              <div className="flex-1 min-w-0">
                <span className="text-xs truncate block text-foreground">
                  {a.name}
                </span>
                <span className="text-[10px] text-muted-foreground">
                  {a.arc_type} · {"★".repeat(a.importance)}
                </span>
              </div>
              <span
                className={`shrink-0 h-1.5 w-1.5 rounded-full ${statusDot(a.status)}`}
              />
            </div>
          ))
        )}
      </div>
    </>
  );
}

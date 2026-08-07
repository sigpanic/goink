import { useMemo, useState } from "react";
import { GitBranch, Circle } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useFocusStore } from "@/stores/useFocusStore";
import SearchInput from "@/components/shared/SearchInput";
import { useStoryArcs } from "./useStoryArcs";
import { useArcNodes } from "./useArcNodes";

interface Props {
  novelId: number;
}

export default function SidebarArcList({ novelId }: Props) {
  const { t } = useTranslation();
  // 4.3.1: arcs 走 useStoryArcs query（与 ArcListView / StoryArcGraph 共享缓存）。
  // 4a: query 错误 toast 由全局中间件接管（queryErrorToast.ts），此处不再挂 useEffect。
  const { data: arcs = [], isError } = useStoryArcs(novelId);
  // 4b: 拉 nodes 数据用于领域内搜索（搜 node title + description）。
  const { data: nodes = [] } = useArcNodes(novelId);
  // 4b: 点击 arc/node 条目触发 focusEntity，ArcListView 的 useEffect 按 type 分流定位。
  const focusEntity = useFocusStore((s) => s.focusEntity);
  const [search, setSearch] = useState("");

  // arcID → name 映射，node 条目显示所属弧线名称。
  const arcNameMap = useMemo(() => {
    const m = new Map<number, string>();
    for (const a of arcs) m.set(a.id, a.name);
    return m;
  }, [arcs]);

  // 搜 arc（name + description）
  const filteredArcs = useMemo(() => {
    if (!search.trim()) return arcs;
    const q = search.toLowerCase();
    return arcs.filter(
      (a) =>
        a.name.toLowerCase().includes(q) ||
        (a.description || "").toLowerCase().includes(q),
    );
  }, [arcs, search]);

  // 搜 node（title + description），仅搜索时显示
  const filteredNodes = useMemo(() => {
    if (!search.trim()) return [];
    const q = search.toLowerCase();
    return nodes.filter(
      (n) =>
        n.title.toLowerCase().includes(q) ||
        (n.description || "").toLowerCase().includes(q),
    );
  }, [nodes, search]);

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
        <SearchInput
          value={search}
          onChange={setSearch}
          placeholder={t("storyarc.searchArcs")}
        />
      </div>
      <div className="flex-1 overflow-y-auto overscroll-contain">
        {isError ? (
          <div className="flex items-center justify-center h-full">
            <p className="text-xs text-destructive">
              {t("storyarc.arcsLoadFailed")}
            </p>
          </div>
        ) : filteredArcs.length === 0 && filteredNodes.length === 0 ? (
          <div className="flex items-center justify-center h-full">
            <p className="text-xs text-muted-foreground">
              {search ? t("storyarc.noMatchingArcs") : t("storyarc.noArcs")}
            </p>
          </div>
        ) : (
          <>
            {/* Arc 条目 */}
            {filteredArcs.map((a) => (
              <div
                key={`arc-${a.id}`}
                onClick={() => focusEntity("storyarcs", a.id, "arc")}
                className="w-full flex items-center gap-2 px-3 py-1.5 text-left cursor-pointer hover:bg-muted transition-colors group"
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
            ))}
            {/* Node 条目（仅搜索时显示） */}
            {search.trim() &&
              filteredNodes.map((n) => (
                <div
                  key={`node-${n.id}`}
                  onClick={() => focusEntity("storyarcs", n.id, "node")}
                  className="w-full flex items-center gap-2 px-3 py-1.5 text-left cursor-pointer hover:bg-muted transition-colors group"
                >
                  <span className="shrink-0 flex h-5 w-5 items-center justify-center rounded bg-tag-blue text-tag-blue-foreground">
                    <Circle className="h-3 w-3" />
                  </span>
                  <div className="flex-1 min-w-0">
                    <span className="text-xs truncate block text-foreground">
                      {n.title}
                    </span>
                    <span className="text-[10px] text-muted-foreground truncate block">
                      {arcNameMap.get(n.story_arc_id) || ""} ·{" "}
                      {t("sidebar.chapterN", { n: n.target_chapter })}
                    </span>
                  </div>
                </div>
              ))}
          </>
        )}
      </div>
    </>
  );
}

import { useState, useMemo } from "react";
import { Search, Settings } from "lucide-react";
import { useTranslation } from "react-i18next";
import { usePreferences } from "./usePreferences";

interface Props {
  novelId: number;
}

export default function SidebarPreferenceList({ novelId }: Props) {
  const { t } = useTranslation();
  // 4.6.1: preferences 走 query（与 PreferenceView 共享缓存）。
  // 4a: query 错误 toast 由全局中间件接管，组件加 isError 内连显示（对齐 ReaderList）。
  const { data, isError } = usePreferences(novelId);
  const [search, setSearch] = useState("");

  // PreferenceList 合并 global + novel 两组为一个列表（保持原 load() 行为）。
  const items = useMemo(() => {
    if (!data) return [];
    return [...(data.global ?? []), ...(data.novel ?? [])];
  }, [data]);

  const filtered = useMemo(() => {
    if (!search.trim()) return items;
    const q = search.toLowerCase();
    return items.filter(
      (e) =>
        e.content.toLowerCase().includes(q) ||
        e.category.toLowerCase().includes(q),
    );
  }, [items, search]);

  return (
    <>
      <div className="flex items-center justify-between px-3 py-2.5 border-b">
        <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          {t("preference.creativePreference")} ({items.length})
        </span>
      </div>
      <div className="px-2 py-1.5 border-b">
        <div className="relative">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-3 h-3 text-muted-foreground" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t("preference.searchPreference")}
            className="w-full h-7 rounded-md border bg-background pl-7 pr-2 text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>
      </div>
      <div className="flex-1 overflow-y-auto overscroll-contain">
        {isError ? (
          <div className="flex items-center justify-center h-full">
            <p className="text-xs text-destructive">
              {t("preference.loadFailed")}
            </p>
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex items-center justify-center h-full">
            <p className="text-xs text-muted-foreground">
              {search
                ? t("preference.noMatchingPreference")
                : t("preference.noPreference")}
            </p>
          </div>
        ) : (
          filtered.map((e) => (
            <div
              key={e.id}
              className="w-full flex items-center gap-2 px-3 py-1.5 text-left cursor-pointer hover:bg-muted transition-colors"
            >
              <span className="shrink-0 flex h-5 w-5 items-center justify-center rounded bg-secondary text-muted-foreground">
                <Settings className="h-3 w-3" />
              </span>
              <div className="flex-1 min-w-0">
                <span className="text-xs truncate block text-foreground">
                  {e.content.length > 30
                    ? e.content.slice(0, 30) + "…"
                    : e.content}
                </span>
                <span className="text-[10px] text-muted-foreground">
                  {e.category || t("preference.uncategorized")}
                  {e.is_global
                    ? ` · ${t("preference.global")}`
                    : ` · ${t("preference.book")}`}
                </span>
              </div>
            </div>
          ))
        )}
      </div>
    </>
  );
}

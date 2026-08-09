import { useState, useMemo } from "react";
import { Globe } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useFocusStore } from "@/stores/useFocusStore";
import SearchInput from "@/components/shared/SearchInput";
import { useNovelSettings } from "./useNovelSettings";

interface Props {
  novelId: number;
}

export default function NovelSettingList({ novelId }: Props) {
  const { t } = useTranslation();
  // 4.7.1: settings 走 query（与 NovelSettingView 共享缓存）。
  // 4a: query 错误 toast 由全局中间件接管，组件加 isError 内连显示（对齐 PreferenceList）。
  const { data, isError } = useNovelSettings(novelId);
  const items = data?.items ?? [];
  // 4b: 点击条目触发 focusEntity，NovelSettingView useEffect 定位+高亮。
  const focusEntity = useFocusStore((s) => s.focusEntity);
  const [search, setSearch] = useState("");

  // 4b: filter 字段对齐后端 Search（content OR category）。
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
          {t("novelSetting.title")} ({items.length})
        </span>
      </div>
      <div className="px-2 py-1.5 border-b">
        <SearchInput
          value={search}
          onChange={setSearch}
          placeholder={t("novelSetting.searchSetting")}
        />
      </div>
      <div className="flex-1 overflow-y-auto overscroll-contain">
        {isError ? (
          <div className="flex items-center justify-center h-full">
            <p className="text-xs text-destructive">
              {t("novelSetting.loadFailed")}
            </p>
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex items-center justify-center h-full">
            <p className="text-xs text-muted-foreground">
              {search
                ? t("novelSetting.noMatchingSetting")
                : t("novelSetting.noSetting")}
            </p>
          </div>
        ) : (
          filtered.map((e) => (
            <div
              key={e.id}
              onClick={() => focusEntity("novel-settings", e.id)}
              className="w-full flex items-center gap-2 px-3 py-1.5 text-left cursor-pointer hover:bg-muted transition-colors group"
            >
              <span className="shrink-0 flex h-5 w-5 items-center justify-center rounded bg-secondary text-muted-foreground">
                <Globe className="h-3 w-3" />
              </span>
              <div className="flex-1 min-w-0">
                <span className="text-xs truncate block text-foreground">
                  {e.content.length > 30
                    ? e.content.slice(0, 30) + "…"
                    : e.content}
                </span>
                <span className="text-[10px] text-muted-foreground">
                  {e.category || t("novelSetting.uncategorized")}
                </span>
              </div>
            </div>
          ))
        )}
      </div>
    </>
  );
}

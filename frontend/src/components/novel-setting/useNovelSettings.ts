import { useQuery } from "@tanstack/react-query";
import { GetNovelSettings } from "@/lib/wailsjs/go/app/App";
import { novelSettingKeys } from "@/lib/queryKeys";

// useNovelSettings: 小说设定列表 query（含 items + token 预算）。
// 后端 GetNovelSettings(novelId) 返回 app.SettingResult（含 items/token_count/over_budget，
// 单 items 数组，不区分 global/novel，区别于 preference 的 PreferenceResult 双数组）。
// queryKey 用 ["novel-settings", novelId] 全量缓存，invalidate 一次刷全部。
// NovelSettingView / NovelSettingList 共享缓存：
// - NovelSettingView 取 result.items / result.token_count / result.over_budget
// - NovelSettingList 取 result.items
export function useNovelSettings(novelId: number) {
  return useQuery({
    queryKey: novelSettingKeys.list(novelId),
    queryFn: async () => {
      const result = await GetNovelSettings(novelId);
      return result;
    },
    enabled: !!novelId,
  });
}

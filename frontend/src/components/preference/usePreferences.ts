import { useQuery } from "@tanstack/react-query";
import { GetPreferences } from "@/lib/wailsjs/go/app/App";
import { preferenceKeys } from "@/lib/queryKeys";

// usePreferences: 偏好列表 query（含全局 + 小说级两组 + token 预算）。
// 后端 GetPreferences(novelId) 返回 app.PreferenceResult（含 global/novel/token_count/over_budget），
// 非数组，区别于 reader/timeline 的数组返回。queryKey 用 ["preferences", novelId] 全量缓存，
// invalidate 一次刷全部。
// PreferenceView / PreferenceList 共享缓存：
// - PreferenceView 取 result.global / result.novel / result.token_count / result.over_budget
// - PreferenceList 合并 result.global + result.novel 为一个列表
export function usePreferences(novelId: number) {
  return useQuery({
    queryKey: preferenceKeys.list(novelId),
    queryFn: async () => {
      const result = await GetPreferences(novelId);
      return result;
    },
    enabled: !!novelId,
  });
}

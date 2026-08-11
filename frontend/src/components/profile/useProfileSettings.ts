import { useQuery } from "@tanstack/react-query";
import { GetSettings } from "@/lib/wailsjs/go/app/App";
import { settingsKeys } from "@/lib/queryKeys";
import type { config } from "@/lib/wailsjs/go/models";

// useProfileSettings: 应用设置 query（profile 消费 user_name / avatar 字段）。
// 复用 settingsKeys.all 与 chat 的 useSettings 共享缓存——GetSettings 是全局设置，
// chat 读 selected_model_key / reasoning_effort 等字段，profile 读 user_name / avatar，
// 各取各字段，TanStack Query 只 fetch 一次。
// queryFn 直接 import wailsjs GetSettings（不用 useApp）。
// GET 错误由全局中间件接管（settings 前缀 → chat.settingsLoadFailed，文案通用）。
// 5.7 commit 1。
export type ProfileSettings = config.AppSettings;

export function useProfileSettings() {
  return useQuery({
    queryKey: settingsKeys.all,
    queryFn: async () => {
      return (await GetSettings()) as ProfileSettings;
    },
  });
}

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CreateNovelSetting } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { novelSettingKeys } from "@/lib/queryKeys";

// useCreateNovelSetting: 创建小说设定条目 mutation。
// mutationFn 直接 import wailsjs CreateNovelSetting（不用 useApp），返回 setting.SettingItem。
// 参数顺序：CreateNovelSetting(novelId, input)（同 preference CreatePreference）。
// input 无 is_global 字段（区别于 preference，novel-setting 无全局/小说级区分）。
// 消费方：NovelSettingView.handleSave（mutateAsync 抛错由 handler try/catch 接住）。
// handler 负责 setEditMode(null) + setForm(EMPTY_FORM) + 错误 toast（副作用不放进 mutation）。
// onSuccess 失效 novel-settings：新 entry 入列表，NovelSettingView / NovelSettingList 同步。
export function useCreateNovelSetting(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: app.CreateNovelSettingInput) =>
      CreateNovelSetting(novelId, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: novelSettingKeys.list(novelId) });
    },
  });
}

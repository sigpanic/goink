import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CreatePreference } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { preferenceKeys } from "@/lib/queryKeys";

// useCreatePreference: 创建偏好条目 mutation。
// mutationFn 直接 import wailsjs CreatePreference（不用 useApp），返回 preference.PreferenceItem。
// 参数顺序：CreatePreference(novelId, input)（同 reader CreateReaderPerspective）。
// 消费方：PreferenceView.handleSave（mutateAsync 抛错由 handler try/catch 接住）。
// handler 负责 setEditMode(null) + setForm(EMPTY_FORM) + 错误 toast（副作用不放进 mutation）。
// onSuccess 失效 preferences：新 entry 入列表，PreferenceView / PreferenceList 同步。
export function useCreatePreference(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: app.CreatePreferenceInput) =>
      CreatePreference(novelId, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: preferenceKeys.list(novelId) });
    },
  });
}

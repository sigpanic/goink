import { useMutation, useQueryClient } from "@tanstack/react-query";
import { UpdatePreference } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { preferenceKeys } from "@/lib/queryKeys";

// useUpdatePreference: 更新偏好条目 mutation。
// mutationFn 直接 import wailsjs UpdatePreference（不用 useApp），返回 preference.PreferenceItem。
// 参数顺序：UpdatePreference(novelId, id, input)（3 参，novelId 在前，
// 与 reader 的 UpdateReaderPerspective(id, novelId, input) 顺序相反，preference 后端签名如此）。
// 入参 {id, input}：input 用 app.UpdatePreferenceInput（PUT 语义，全量回传）。
// 消费方：PreferenceView.handleSave（mutateAsync 抛错由 handler try/catch 接住）。
// handler 负责 setEditMode(null) + setForm(EMPTY_FORM) + 错误 toast（副作用不放进 mutation）。
// onSuccess 失效 preferences：entry 字段变更入列表。
export function useUpdatePreference(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      input,
    }: {
      id: number;
      input: app.UpdatePreferenceInput;
    }) => UpdatePreference(novelId, id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: preferenceKeys.list(novelId) });
    },
  });
}

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { DeletePreference } from "@/lib/wailsjs/go/app/App";
import { preferenceKeys } from "@/lib/queryKeys";

// useDeletePreference: 删除偏好条目 mutation。
// mutationFn 直接 import wailsjs DeletePreference（不用 useApp）。
// 单参 API：DeletePreference(preferenceId)（区别于 reader 的双参 entryId+novelId）。
// 消费方：PreferenceView.confirmDelete（mutateAsync 抛错由 handler try/catch 接住）。
// handler 负责 setDeleteTarget(null) + 错误 toast（副作用不放进 mutation）。
// onSuccess 失效 preferences：entry 删除后列表同步（PreferenceView + PreferenceList 共享缓存）。
export function useDeletePreference(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (preferenceId: number) => DeletePreference(preferenceId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: preferenceKeys.list(novelId) });
    },
  });
}

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { DeleteNovelSetting } from "@/lib/wailsjs/go/app/App";
import { novelSettingKeys } from "@/lib/queryKeys";

// useDeleteNovelSetting: 删除小说设定条目 mutation。
// mutationFn 直接 import wailsjs DeleteNovelSetting（不用 useApp）。
// 单参 API：DeleteNovelSetting(settingId)（同 preference 的 DeletePreference(id)，区别于 reader 双参）。
// 消费方：NovelSettingView.confirmDelete（mutateAsync 抛错由 handler try/catch 接住）。
// handler 负责 setDeleteTarget(null) + 错误 toast（副作用不放进 mutation）。
// onSuccess 失效 novel-settings：entry 删除后列表同步（NovelSettingView + NovelSettingList 共享缓存）。
export function useDeleteNovelSetting(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (settingId: number) => DeleteNovelSetting(settingId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: novelSettingKeys.list(novelId) });
    },
  });
}

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { UpdateNovelSetting } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { novelSettingKeys } from "@/lib/queryKeys";

// useUpdateNovelSetting: 更新小说设定条目 mutation。
// mutationFn 直接 import wailsjs UpdateNovelSetting（不用 useApp），返回 setting.SettingItem。
// 参数顺序：UpdateNovelSetting(novelId, id, input)（3 参，novelId 在前，同 preference，
// 与 reader 的 UpdateReaderPerspective(id, novelId, input) 顺序相反）。
// input 无 is_global 字段（区别于 preference，novel-setting 无全局/小说级区分）。
// 入参 {id, input}：input 用 app.UpdateNovelSettingInput（全 optional，PATCH 语义），
// 但 handler 全量回传 input 所有字段（§6 等价 PUT）。
// 消费方：NovelSettingView.handleSave（mutateAsync 抛错由 handler try/catch 接住）。
// handler 负责 setEditMode(null) + setForm(EMPTY_FORM) + 错误 toast（副作用不放进 mutation）。
// onSuccess 失效 novel-settings：entry 字段变更入列表。
export function useUpdateNovelSetting(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      input,
    }: {
      id: number;
      input: app.UpdateNovelSettingInput;
    }) => UpdateNovelSetting(novelId, id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: novelSettingKeys.list(novelId) });
    },
  });
}

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { UpdateReaderPerspective } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { readerKeys } from "@/lib/queryKeys";

// useUpdateReaderPerspective: 更新读者视角条目 mutation。
// mutationFn 直接 import wailsjs UpdateReaderPerspective（不用 useApp）。
// 参数顺序：UpdateReaderPerspective(entryId, novelId, input)（3 参，entryId 在前，
// 与 timeline 的 UpdateTimelineEntry(novelId, id, input) 顺序不同，reader 后端签名如此）。
// 入参 {id, input}：input 用 app.UpdateReaderPerspectiveInput（PUT 语义，全量回传），含 handleQuickReveal 全量回传。
// 消费方：ReaderView.handleUpdate / handleQuickReveal（mutateAsync 抛错由 handler try/catch 接住）。
// handler 负责 setEditMode(null) / 错误 toast（副作用各异，不放进 mutation）。
// onSuccess 失效 reader：entry 字段变更入列表。
export function useUpdateReaderPerspective(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      input,
    }: {
      id: number;
      input: app.UpdateReaderPerspectiveInput;
    }) => UpdateReaderPerspective(id, novelId, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: readerKeys.list(novelId) });
    },
  });
}

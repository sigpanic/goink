import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CreateReaderPerspective } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { readerKeys } from "@/lib/queryKeys";

// useCreateReaderPerspective: 创建读者视角条目 mutation。
// mutationFn 直接 import wailsjs CreateReaderPerspective（不用 useApp），返回 reader.ReaderPerspective。
// 参数顺序：CreateReaderPerspective(novelId, input)（同 timeline）。
// 消费方：ReaderView.handleCreate（mutateAsync 抛错由 handler try/catch 接住）。
// handler 负责 setEditMode(null) + setForm(EMPTY_FORM) + setExpandedId(created.id) + 错误 toast（副作用各异，不放进 mutation）。
// onSuccess 失效 reader：新 entry 入列表，ReaderView / ReaderList 同步。
export function useCreateReaderPerspective(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: app.CreateReaderPerspectiveInput) =>
      CreateReaderPerspective(novelId, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: readerKeys.list(novelId) });
    },
  });
}

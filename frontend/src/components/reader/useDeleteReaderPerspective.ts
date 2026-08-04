import { useMutation, useQueryClient } from "@tanstack/react-query";
import { DeleteReaderPerspective } from "@/lib/wailsjs/go/app/App";
import { readerKeys } from "@/lib/queryKeys";

// useDeleteReaderPerspective: 删除读者视角条目 mutation。
// mutationFn 直接 import wailsjs DeleteReaderPerspective（不用 useApp）。
// 注意参数顺序：DeleteReaderPerspective(entryId, novelId)，与 timeline 的
// DeleteTimelineEntry(novelId, entryId) 相反（reader 后端签名如此）。
// 消费方：ReaderView.confirmDelete（mutateAsync 抛错由 handler try/catch 接住）。
// handler 负责 setDeleteTarget(null) + 错误 toast（副作用不放进 mutation）。
// onSuccess 失效 reader：entry 删除后列表同步（ReaderView + ReaderList 共享缓存）。
export function useDeleteReaderPerspective(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (entryId: number) => DeleteReaderPerspective(entryId, novelId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: readerKeys.list(novelId) });
    },
  });
}

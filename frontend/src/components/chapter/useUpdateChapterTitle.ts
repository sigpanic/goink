import { useMutation, useQueryClient } from "@tanstack/react-query";
import { UpdateChapterTitle } from "@/lib/wailsjs/go/app/App";
import { chapterKeys } from "@/lib/queryKeys";

// useUpdateChapterTitle: 更新章节标题 mutation。
// mutationFn 直接 import wailsjs UpdateChapterTitle（不用 useApp），3 参 (novelId, chapter_number, title)。
// §6 全量回传：入参 {chapterNumber, title} 完整传入（UpdateChapterTitle 入参就是这两个，无 patch 语义）。
// onSuccess 失效 chapterKeys.list（ChapterList 标题刷新）。
// 调用方 commitEdit 负责 setEditingId(null) + try/catch + toastError。
export function useUpdateChapterTitle(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      chapterNumber,
      title,
    }: {
      chapterNumber: number;
      title: string;
    }) => UpdateChapterTitle(novelId, chapterNumber, title),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: chapterKeys.list(novelId) });
    },
  });
}

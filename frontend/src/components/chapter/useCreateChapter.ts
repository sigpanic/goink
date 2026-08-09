import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CreateChapter } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { chapterKeys, maxChapterKeys } from "@/lib/queryKeys";

// useCreateChapter: 创建章节 mutation。
// mutationFn 直接 import wailsjs CreateChapter（不用 useApp），单参 input（含 novel_id + title）。
// onSuccess 失效 chapterKeys.list（ChapterList 刷新）+ maxChapterKeys.detail（必做，storyarc 4.3 遗留：
// 新建章节后 storyarc 章节窗口中心 windowCenter 需更新，否则窗口偏移）。
// 调用方 handleCreateChapter 负责 setChapterTitle("")/setShowCreateChapter(false)/setCreateError + try/catch。
export function useCreateChapter(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: app.CreateChapterInput) => CreateChapter(input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: chapterKeys.list(novelId) });
      qc.invalidateQueries({ queryKey: maxChapterKeys.detail(novelId) });
    },
  });
}

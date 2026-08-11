import { useMutation } from "@tanstack/react-query";
import { RebuildNovelIndex } from "@/lib/wailsjs/go/app/App";

// useRebuildNovelIndex: 重建小说向量索引 mutation（命令操作）。
// onSuccess 无需 invalidate（搜索 query staleTime=0，用户下次搜索自动 refetch；
// 重建索引不改变已缓存的 novel/chapter 等元数据）。
// 错误处理由调用方 try/catch + toastError（保留组件级 toast，settings.rebuildFailed）。
// 5.8 commit 2。
export function useRebuildNovelIndex() {
  return useMutation({
    mutationFn: (novelId: number) => RebuildNovelIndex(novelId),
  });
}

import { useQuery } from "@tanstack/react-query";
import { GetCommitFileList } from "@/lib/wailsjs/go/app/App";
import { commitFileKeys } from "@/lib/queryKeys";

// useCommitFiles: 单个 commit 的文件列表（展开 commit 时按需拉取）。
// enabled 守卫：!!hash 才 fetch（hash 为空时不查询）。
// 返回 FileEntry[]（已剥 CommitFileListResult.files）。
//
// 5.4 commit 5：GET 错误由全局中间件接管（queryErrorToast.ts），组件不挂 toastError，
// 但 expandedError inline 保留（commit 展开失败的具体文案由组件读 isError 内连显示）。
export function useCommitFiles(novelId: number, hash: string | null) {
  return useQuery({
    queryKey: commitFileKeys.list(novelId, hash ?? ""),
    queryFn: async () => {
      const result = await GetCommitFileList(novelId, hash!);
      return result?.files ?? [];
    },
    enabled: !!hash,
  });
}

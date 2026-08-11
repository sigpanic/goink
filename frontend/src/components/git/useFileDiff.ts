import { useQuery } from "@tanstack/react-query";
import { GetFileDiff } from "@/lib/wailsjs/go/app/App";
import { fileDiffKeys } from "@/lib/queryKeys";

// useFileDiff: 单个文件的 diff 内容（选中文件时按需拉取）。
// enabled 守卫：!!hash && !!filePath 才 fetch。
//
// 5.4 commit 5：GET 错误由全局中间件接管（queryErrorToast.ts），组件不挂 toastError。
// diff 数据通过 query data 推导，组件 useEffect 监听 data 变化时调 onSelectFile 上传父组件。
export function useFileDiff(
  novelId: number,
  hash: string | null,
  filePath: string | null,
) {
  return useQuery({
    queryKey: fileDiffKeys.detail(novelId, hash ?? "", filePath ?? ""),
    queryFn: () => GetFileDiff(novelId, hash!, filePath!),
    enabled: !!hash && !!filePath,
  });
}

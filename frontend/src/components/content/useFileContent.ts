import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { GetContent } from "@/lib/wailsjs/go/app/App";
import { contentKeys } from "@/lib/queryKeys";

// useFileContent: GetContent 的 fetch 缓存通道。
//
// 设计依据（5.2 特殊点 4）：ContentPanel 多 tab 编辑器，tabs 是本地 state（useEditorTabs），
// GetContent 的角色是"按需 fetch 后塞进 tab.content"——query 是 fetch 缓存通道而非直接驱动 UI。
// fetch 回来的数据仍要 updateTab(tabId, { content }) 回填 tab。
//
// 因调用点都在回调/effect 内（doOpenFile/handleSetViewMode/handleDiffApprove/恢复 tab effect），
// 不能用 useQuery hook（hook 必须在组件顶层），改用 queryClient.fetchQuery：
// - 同 path 30s 内走缓存（继承全局 staleTime），多 tab 共享避免重复 fetch
// - queryFn 直接 import wailsjs GetContent（不用 useApp）
// - query 错误走全局中间件（fetchQuery 创建 query 加入 QueryCache，error state 触发 toast）
// - 调用方 catch 内做兜底（如 updateTab 塞 content.loadFailedCloseTab），保留原 behavior
//
// file:changed 事件触发的强制刷新不走此 hook（commit 1 直接 import GetContent，commit 3 改 qc.invalidateQueries）。
export function useFileContent() {
  const qc = useQueryClient();
  const fetchContent = useCallback(
    async (novelId: number, filePath: string): Promise<string> => {
      return qc.fetchQuery({
        queryKey: contentKeys.detail(novelId, filePath),
        queryFn: () => GetContent(novelId, filePath),
      });
    },
    [qc],
  );
  return { fetchContent };
}

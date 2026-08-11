import { useMutation, useQueryClient } from "@tanstack/react-query";
import { SetLastSession } from "@/lib/wailsjs/go/app/App";
import { settingsKeys } from "@/lib/queryKeys";

// useSetLastSession: 设置上次会话 ID mutation（持久化到 AppSettings.last_session_id）。
// onSuccess invalidate settingsKeys.all —— useSettings refetch 拿新 last_session_id，
// 保持缓存与后端同步。
// 错误处理由调用方 try/catch + toastError。
// 5.1 commit 4 补遗。
export function useSetLastSession() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (sessionId: string) => SetLastSession(sessionId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: settingsKeys.all });
    },
  });
}

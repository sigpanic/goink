import { useMutation, useQueryClient } from "@tanstack/react-query";
import { SetApprovalMode } from "@/lib/wailsjs/go/app/App";
import { settingsKeys } from "@/lib/queryKeys";

// useSetApprovalMode: 设置审批模式 mutation（持久化到 AppSettings.approval_mode）。
// onSuccess invalidate settingsKeys.all —— useSettings refetch 拿新 approval_mode。
// 错误处理由调用方 try/catch + toastError。
// 5.1 commit 4 补遗。
export function useSetApprovalMode() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (mode: string) => SetApprovalMode(mode),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: settingsKeys.all });
    },
  });
}

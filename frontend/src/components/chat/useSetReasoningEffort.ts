import { useMutation, useQueryClient } from "@tanstack/react-query";
import { SetReasoningEffort } from "@/lib/wailsjs/go/app/App";
import { settingsKeys } from "@/lib/queryKeys";

// useSetReasoningEffort: 设置推理强度 mutation（持久化到 AppSettings.reasoning_effort）。
// onSuccess invalidate settingsKeys.all —— useSettings refetch 拿新 reasoning_effort。
// 错误处理由调用方 try/catch + toastError。
// 5.1 commit 4 补遗。
export function useSetReasoningEffort() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (effort: string) => SetReasoningEffort(effort),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: settingsKeys.all });
    },
  });
}

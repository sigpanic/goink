import { useMutation, useQueryClient } from "@tanstack/react-query";
import { SetSelectedModel } from "@/lib/wailsjs/go/app/App";
import { settingsKeys } from "@/lib/queryKeys";

// useSetSelectedModel: 设置选中模型 mutation（持久化 selected_model_key + reasoning_effort 到 AppSettings）。
// 后端 API 签名是 SetSelectedModel(key, effort)，mutationFn 包装成对象 { key, effort }。
// onSuccess invalidate settingsKeys.all —— useSettings refetch 拿新 selected_model_key/reasoning_effort。
// 错误处理由调用方 try/catch + toastError。
// 5.1 commit 4 补遗。
export function useSetSelectedModel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { key: string; effort: string }) =>
      SetSelectedModel(input.key, input.effort),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: settingsKeys.all });
    },
  });
}

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { SaveLLMConfig } from "@/lib/wailsjs/go/app/App";
import type { llm } from "@/lib/wailsjs/go/models";
import { modelKeys, llmConfigKeys } from "@/lib/queryKeys";

// useSaveLLMConfig: 保存 LLM 配置 mutation。
// onSuccess invalidate modelKeys.all（ChatPanel 的 useModels 自动 refetch，选中态由 modelsQuery effect 修正）
// + llmConfigKeys.all（ModelConfigTab 的 useLLMConfig 自动 refetch，替代原 onSaved 回调链路）。
// 错误处理由调用方 try/catch + setSaveMsg（保留组件级 inline 错误展示）。
export function useSaveLLMConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: llm.LLMConfigView) => SaveLLMConfig(input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: modelKeys.all });
      qc.invalidateQueries({ queryKey: llmConfigKeys.all });
    },
  });
}

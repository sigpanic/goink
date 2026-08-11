import { useQuery } from "@tanstack/react-query";
import { GetLLMConfig } from "@/lib/wailsjs/go/app/App";
import { llmConfigKeys } from "@/lib/queryKeys";
import type { llm } from "@/lib/wailsjs/go/models";

// useLLMConfig: LLM 配置 query（含 providers 列表）。
// queryFn 直接 import wailsjs GetLLMConfig（不用 useApp）。
// ModelConfigTab 启动初始化依赖本 query data ready。
// useSaveLLMConfig onSuccess invalidate 此 key 触发 refetch。
// GET 错误由全局中间件接管（llm-config 前缀 → settings.llmConfigLoadFailed）。
// 5.8 commit 1。
export type LLMConfig = llm.LLMConfigView;

export function useLLMConfig() {
  return useQuery({
    queryKey: llmConfigKeys.all,
    queryFn: async () => {
      const config = await GetLLMConfig();
      return config;
    },
  });
}

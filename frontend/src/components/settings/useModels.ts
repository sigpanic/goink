import { useQuery } from "@tanstack/react-query";
import { GetModels } from "@/lib/wailsjs/go/app/App";
import { modelKeys } from "@/lib/queryKeys";

// useModels: 可用模型列表 query。
// queryFn 直接 import wailsjs GetModels（不用 useApp）。
// 全局配置（无 novelId 维度），ChatPanel 是首个消费方；后续 settings 领域迁移时共享缓存。
// 不设 staleTime（继承全局 30s）；ChatPanel 选中态恢复依赖本 query data ready。
export function useModels() {
  return useQuery({
    queryKey: modelKeys.all,
    queryFn: async () => {
      const list = await GetModels();
      return list ?? [];
    },
  });
}

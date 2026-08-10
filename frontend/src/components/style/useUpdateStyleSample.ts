import { useMutation, useQueryClient } from "@tanstack/react-query";
import { UpdateStyleSample } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { styleSampleKeys } from "@/lib/queryKeys";

// useUpdateStyleSample: 更新风格样本 mutation。
// mutationFn 直接 import wailsjs UpdateStyleSample（不用 useApp）。
// 入参 app.UpdateStyleSampleInput：id/name/content/tags/is_global/novel_id 全量回传（§6）。
// handler 负责 setDetailId(null) + 错误 throw（副作用各异，不放进 mutation）。
// onSuccess 失效 list（all 前缀）+ detail（当前 id）：列表刷新 + detail 缓存更新（下次打开拿新数据）。
// mutation 不挂 onError；调用方 try/catch + setError（对齐 character 模式）。
export function useUpdateStyleSample() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: app.UpdateStyleSampleInput) =>
      UpdateStyleSample(input),
    onSuccess: (_data, variables) => {
      qc.invalidateQueries({ queryKey: styleSampleKeys.all });
      qc.invalidateQueries({ queryKey: styleSampleKeys.detail(variables.id) });
    },
  });
}

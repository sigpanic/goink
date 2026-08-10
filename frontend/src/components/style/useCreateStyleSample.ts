import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CreateStyleSample } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { styleSampleKeys } from "@/lib/queryKeys";

// useCreateStyleSample: 创建风格样本 mutation。
// mutationFn 直接 import wailsjs CreateStyleSample（不用 useApp）。
// 入参 app.CreateStyleSampleInput：novel_id/is_global/name/content/tags。
// handler 负责 setNewName/reset form/setPage(1) + 错误 throw（副作用各异，不放进 mutation）。
// onSuccess 失效 styleSampleKeys.all：新样本入列表，StyleView/StyleSampleList 缓存同步。
// mutation 不挂 onError；调用方 try/catch + setError（对齐 character 模式）。
export function useCreateStyleSample() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: app.CreateStyleSampleInput) =>
      CreateStyleSample(input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: styleSampleKeys.all });
    },
  });
}

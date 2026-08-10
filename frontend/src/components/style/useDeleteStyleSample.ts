import { useMutation, useQueryClient } from "@tanstack/react-query";
import { DeleteStyleSample } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { styleSampleKeys } from "@/lib/queryKeys";

// useDeleteStyleSample: 删除风格样本 mutation。
// mutationFn 直接 import wailsjs DeleteStyleSample（不用 useApp）。
// 入参 app.DeleteStyleSampleInput：{ id }。
// handler 负责 setSelected 过滤 + setDeleteTarget(null) + 错误 throw（副作用各异，不放进 mutation）。
// onSuccess 失效 styleSampleKeys.all：列表刷新（StyleView/StyleSampleList 同步）。
// mutation 不挂 onError；调用方 try/catch + toastError（对齐 character 模式）。
export function useDeleteStyleSample() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: app.DeleteStyleSampleInput) =>
      DeleteStyleSample(input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: styleSampleKeys.all });
    },
  });
}

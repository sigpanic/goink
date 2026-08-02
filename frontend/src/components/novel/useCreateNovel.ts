import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CreateNovel } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { novelKeys } from "@/lib/queryKeys";

// useCreateNovel: 创建小说 mutation。
// mutationFn 直接 import wailsjs CreateNovel（不用 useApp），onSuccess 失效 novelKeys.all。
// handler 负责 switchToNovel + 关 UI（副作用各异，不放进 mutation）。
// 入参类型 app.CreateNovelInput：title 必填，description/genre 可选。
export function useCreateNovel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: app.CreateNovelInput) => CreateNovel(input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: novelKeys.all });
    },
  });
}

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CreateArcNode } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { arcNodeKeys } from "@/lib/queryKeys";

// useCreateArcNode: 创建弧线节点 mutation。
// mutationFn 直接 import wailsjs CreateArcNode（不用 useApp），返回 storyarc.ArcNode。
// handler 用返回值 created.id 做 setExpandedId（副作用各异，不放进 mutation）。
// onSuccess 失效 arc-nodes：新 node 入列表，ArcListView / StoryArcGraph 同步。
// 不失效 storyarcs：新建 node 不影响 arc 列表。
export function useCreateArcNode(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: app.CreateArcNodeInput) =>
      CreateArcNode(novelId, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: arcNodeKeys.list(novelId) });
    },
  });
}

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CreateStoryArc } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { storyarcKeys } from "@/lib/queryKeys";

// useCreateStoryArc: 创建故事弧线 mutation。
// mutationFn 直接 import wailsjs CreateStoryArc（不用 useApp），返回 storyarc.StoryArc。
// handler 负责 setEditMode(null) + 错误 throw（副作用各异，不放进 mutation）。
// onSuccess 失效 storyarcs：新 arc 入列表，ArcList / ArcListView / StoryArcGraph 同步。
// 不失效 arc-nodes：新建 arc 无 node，arc-nodes 缓存无脏数据。
export function useCreateStoryArc(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: app.CreateStoryArcInput) =>
      CreateStoryArc(novelId, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: storyarcKeys.list(novelId) });
    },
  });
}

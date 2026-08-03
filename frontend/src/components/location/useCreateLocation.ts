import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CreateLocation } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { locationKeys } from "@/lib/queryKeys";

// useCreateLocation: 创建地点 mutation（参考 useCreateCharacter）。
// mutationFn 直接 import wailsjs CreateLocation（不用 useApp）。
// 入参 app.CreateLocationInput：name 必填，location_type/description/parent_location_id/tags optional。
// handler 负责 setEditMode(null) + 错误 throw（副作用各异，不放进 mutation）。
// onSuccess 失效 list：新地点入列表，LocationListView / LocationList / LocationGraph 节点同步。
// 不失效 relations：新建地点无关系，relations 缓存无脏数据。
export function useCreateLocation(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: app.CreateLocationInput) =>
      CreateLocation(novelId, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: locationKeys.list(novelId) });
    },
  });
}

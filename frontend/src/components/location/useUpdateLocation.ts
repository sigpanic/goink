import { useMutation, useQueryClient } from "@tanstack/react-query";
import { UpdateLocation } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { locationKeys } from "@/lib/queryKeys";

// useUpdateLocation: 更新地点 mutation（参考 useUpdateCharacter）。
// mutationFn 直接 import wailsjs UpdateLocation（不用 useApp）。
// 入参 {id, input}：input 用 app.UpdateLocationInput（PUT 语义，全量回传）。
// handler 负责 setEditMode(null) + 错误 throw（副作用各异，不放进 mutation）。
// onSuccess 失效 list：改名/改描述/改类型/改 parent 后列表同步（LocationList 树形结构 + Graph 节点同步重建）。
// 不失效 relations：LocationRelation 只存 location_a/location_b 不存 name，
// 改名/改描述/改类型/改 parent 不影响 relations 缓存（Graph 节点名来自 useLocations 的 refetch）。
export function useUpdateLocation(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      input,
    }: {
      id: number;
      input: app.UpdateLocationInput;
    }) => UpdateLocation(novelId, id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: locationKeys.list(novelId) });
    },
  });
}

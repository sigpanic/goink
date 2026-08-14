import { useMutation, useQueryClient } from "@tanstack/react-query";
import { UpdateCharacter } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { characterKeys } from "@/lib/queryKeys";

// useUpdateCharacter: 更新角色 mutation。
// mutationFn 直接 import wailsjs UpdateCharacter（不用 useApp）。
// 入参 {id, input}：input 用 app.UpdateCharacterInput（PUT 语义，全量回传）。
// handler 负责 setEditMode(null) + 错误 throw（副作用各异，不放进 mutation）。
// onSuccess 失效 list：角色名/描述/能力更新入列表。
// 不失效 relations：CharacterRelation 只存 character_id 不存 name，
// 改名/改描述/改能力不影响 relations 缓存（Graph 节点名来自 useCharacters 的 refetch）。
export function useUpdateCharacter(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      input,
    }: {
      id: number;
      input: app.UpdateCharacterInput;
    }) => UpdateCharacter(novelId, id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: characterKeys.list(novelId) });
    },
  });
}

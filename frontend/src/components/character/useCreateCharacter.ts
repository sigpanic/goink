import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CreateCharacter } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { characterKeys } from "@/lib/queryKeys";

// useCreateCharacter: 创建角色 mutation。
// mutationFn 直接 import wailsjs CreateCharacter（不用 useApp）。
// 入参 app.CreateCharacterInput：name 必填，description/personality/abilities optional。
// handler 负责 setEditMode(null) + 错误 throw（副作用各异，不放进 mutation）。
// onSuccess 失效 list：新角色入列表，CharacterListView / CharacterList / CharacterGraph 节点同步。
// 不失效 relations：新建角色无关系，relations 缓存无脏数据。
export function useCreateCharacter(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: app.CreateCharacterInput) =>
      CreateCharacter(novelId, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: characterKeys.list(novelId) });
    },
  });
}

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  GetCharacterGroups,
  GetCharacterGroupMembers,
  CreateCharacterGroup,
  UpdateCharacterGroup,
  DeleteCharacterGroup,
  UpdateCharacterGroupMemberships,
} from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { characterKeys } from "@/lib/queryKeys";

// 树导航和角色卡片共用完整分类数据，与现有全量角色缓存配套。
export function useCharacterGroups(novelId: number) {
  return useQuery({
    queryKey: characterKeys.groups(novelId),
    queryFn: async () => {
      const result = await GetCharacterGroups({
        novel_id: novelId,
        page: 1,
        size: -1,
        search: "",
      });
      return result.items;
    },
    enabled: novelId > 0,
  });
}

export function useCharacterGroupMembers(novelId: number) {
  return useQuery({
    queryKey: characterKeys.memberships(novelId),
    queryFn: async () => {
      const result = await GetCharacterGroupMembers({
        novel_id: novelId,
        page: 1,
        size: -1,
        search: "",
        group_id: 0,
        character_id: 0,
      });
      return result.items;
    },
    enabled: novelId > 0,
  });
}

export function useCharacterGroupMutations(novelId: number) {
  const qc = useQueryClient();
  const refresh = () =>
    Promise.all([
      qc.invalidateQueries({ queryKey: characterKeys.groups(novelId) }),
      qc.invalidateQueries({ queryKey: characterKeys.memberships(novelId) }),
    ]);
  const create = useMutation({
    mutationFn: (input: app.CharacterGroupInput) =>
      CreateCharacterGroup(novelId, input),
    onSuccess: refresh,
  });
  const update = useMutation({
    mutationFn: ({
      id,
      input,
    }: {
      id: number;
      input: app.CharacterGroupInput;
    }) => UpdateCharacterGroup(novelId, id, input),
    onSuccess: refresh,
  });
  const remove = useMutation({
    mutationFn: (id: number) => DeleteCharacterGroup(novelId, id),
    onSuccess: refresh,
  });
  const memberships = useMutation({
    mutationFn: (input: app.CharacterGroupMembershipInput) =>
      UpdateCharacterGroupMemberships(novelId, input),
    onSuccess: refresh,
  });
  return { create, update, remove, memberships };
}

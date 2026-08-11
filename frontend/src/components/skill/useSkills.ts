import { useQuery } from "@tanstack/react-query";
import { ListSkills } from "@/lib/wailsjs/go/app/App";
import { skillKeys } from "@/lib/queryKeys";

// useSkills: 技能列表 query。
// queryFn 直接 import wailsjs ListSkills（不用 useApp），不设 staleTime（继承全局 30s）。
// enabled: !!novelId 守卫，novelId=0 时不 fetch（数据兜底空数组）。
// 消费方：SkillList / SkillMarketplace（已安装索引，commit 3 迁）共享缓存，
// CRUD 后由 mutation 的 invalidateQueries 同步（commit 2 迁 useDeleteSkill）。
// ListSkills 是旧 API（直接返回数组，非 apperr.Result），无需 unwrapResult。
export function useSkills(novelId: number) {
  return useQuery({
    queryKey: skillKeys.list(novelId),
    queryFn: async () => {
      const list = await ListSkills({ novel_id: novelId });
      return list ?? [];
    },
    enabled: !!novelId,
  });
}

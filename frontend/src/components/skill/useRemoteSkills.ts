import { useQuery } from "@tanstack/react-query";
import { ListRemoteSkills } from "@/lib/wailsjs/go/app/App";
import type { app, storage, remote } from "@/lib/wailsjs/go/models";
import { skillKeys } from "@/lib/queryKeys";
import { unwrapResult } from "@/utils/wailsResult";

// useRemoteSkills: 远程技能市场列表 query（apperr 新 API）。
// queryFn 用 unwrapResult 解包 Result[PageResult[RemoteSkillMeta]]，
// err_code 非空时 throw AppErr（带 errCode），query 进 error 状态触发中间件兜底 toast。
// 组件读 query.error（AppErr）的 errCode，传给 classifyError 算 inline 具体文案。
// enabled 由调用方控制（SkillMarketplace 在 open 时才 fetch）。
// debounce 由调用方控制 debouncedQuery 进 queryKey，queryKey 变化自动 refetch。
//
// T 明确为 PageResult 结构类型（unwrapResult 泛型推断会退化为 {}，需手动标注）。
type RemoteSkillPageResult = {
  items: remote.RemoteSkillMeta[];
  total: number;
  total_pages: number;
};

export function useRemoteSkills(
  input: app.ListRemoteSkillsInput,
  enabled: boolean,
) {
  return useQuery({
    queryKey: skillKeys.remoteList(input),
    queryFn: async (): Promise<RemoteSkillPageResult> => {
      const res = await ListRemoteSkills(input);
      return unwrapResult<storage.PageResult_github_com_sigpanic_goink_internal_skill_remote_RemoteSkillMeta_>(res);
    },
    enabled,
  });
}

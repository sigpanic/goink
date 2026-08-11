import { useQuery } from "@tanstack/react-query";
import { GetRemoteSkillContent } from "@/lib/wailsjs/go/app/App";
import { skillKeys } from "@/lib/queryKeys";
import { unwrapResult } from "@/utils/wailsResult";

// fetchRemoteSkillContent: 拉取远程技能内容（apperr 新 API）。
// 导出供 SkillMarketplace.handleInstall 在 confirm_overwrite 流程中复用：
// detail phase 已通过 useRemoteSkillContent query 缓存，handleInstall 走 qc.fetchQuery
// 同 queryKey 复用缓存，避免重复拉取。
export async function fetchRemoteSkillContent(name: string): Promise<string> {
  const res = await GetRemoteSkillContent(name);
  return unwrapResult(res);
}

// useRemoteSkillContent: 远程技能内容 query（apperr 新 API）。
// queryFn 用 unwrapResult 解包 Result[string]，err_code 非空时 throw AppErr。
// enabled: !!name && phase === "detail"（调用方传），confirm_overwrite phase 不重新 fetch
// （用 handleInstall 时拷贝到 remoteContentForConfirm 的缓存值）。
export function useRemoteSkillContent(name: string, enabled: boolean) {
  return useQuery({
    queryKey: skillKeys.remoteContent(name),
    queryFn: () => fetchRemoteSkillContent(name),
    enabled,
  });
}

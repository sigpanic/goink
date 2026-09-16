import { useMutation, useQueryClient } from "@tanstack/react-query";
import { InstallRemoteSkill } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { contentKeys, skillKeys } from "@/lib/queryKeys";
import { unwrapResult } from "@/utils/wailsResult";

// useInstallRemoteSkill: 安装远程技能 mutation（apperr 新 API）。
// mutationFn 用 unwrapResult 解包 Result[struct]，err_code 非空时 throw AppErr
// （data 是空 struct 无意义，仅消费 err_code/err_msg）。
// 入参 app.InstallRemoteSkillInput（含 name/target/novel_id），调用方拼全字段传入
// （与 useDeleteSkill 模式对齐：novelId 从 hook 闭包已知，但 InstallRemoteSkillInput
// 后端绑定要求 novel_id 字段，由调用方在 input 内补齐）。
// onSuccess 失效：
//   - skillKeys.list(novelId)：SkillList/SkillMarketplace 已安装索引刷新，
//     installedVersions Map 重算（决定卡片 installed/updatable 标记）
//   - ["remote-skills"]：远程列表整体失效，卡片标记刷新
//   - contentKeys.detail：清除安装前 probeLocal 对同路径 GetContent 缓存下的空内容
//     （issue #47：安装后点开技能预览无内容，重启才恢复）
// mutation 不挂 onError；调用方 try/catch + toastError（对齐 useDeleteSkill 模式）。
// AppErr.errCode 由调用方读出经 classifyError 映射短码文案（如 rate_limited → errorRateLimited）。
export function useInstallRemoteSkill(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: app.InstallRemoteSkillInput) => {
      const res = await InstallRemoteSkill(input);
      return unwrapResult(res);
    },
    onSuccess: (_data, input) => {
      qc.invalidateQueries({ queryKey: skillKeys.list(novelId) });
      qc.invalidateQueries({ queryKey: ["remote-skills"] });
      // 安装路径与 SkillList.skillPath / SkillMarketplace.pathForSource 保持一致
      const path =
        input.target === "novel"
          ? `skills/${input.name}.md`
          : `~/.goink/skills/${input.name}.md`;
      qc.invalidateQueries({
        queryKey: contentKeys.detail(input.novel_id, path),
      });
    },
  });
}

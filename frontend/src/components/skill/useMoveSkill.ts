import { useMutation, useQueryClient } from "@tanstack/react-query";
import { MoveSkill } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { contentKeys, skillKeys } from "@/lib/queryKeys";
import { unwrapResult } from "@/utils/wailsResult";

// useMoveSkill: 技能换层 mutation（apperr 新 API）。
// mutationFn 用 unwrapResult 解包 Result[SkillMeta]，err_code 非空时 throw AppErr，
// 调用方读 AppErr.errCode 映射具体文案（skill.exists / skill.not_found 等）。
// onSuccess 失效 skillKeys.all：源层或目标层必有一侧是用户级，
// 用户级技能对所有小说可见，因此所有小说的技能列表都需要刷新。
// 同时清理源/目标路径的 content 缓存：文件已换路径，旧缓存不可复用。
export function useMoveSkill(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: app.MoveSkillInput) =>
      unwrapResult(await MoveSkill(input)),
    onSuccess: (_meta, input) => {
      qc.invalidateQueries({ queryKey: skillKeys.all });
      const oldPath =
        input.from === "user"
          ? `~/.goink/skills/${input.name}.md`
          : `skills/${input.name}.md`;
      const newPath =
        input.to === "user"
          ? `~/.goink/skills/${input.name}.md`
          : `skills/${input.name}.md`;
      qc.removeQueries({
        predicate: ({ queryKey }) =>
          queryKey[0] === contentKeys.all[0] &&
          (queryKey[2] === oldPath || queryKey[2] === newPath) &&
          (input.from === "user" ||
            input.to === "user" ||
            queryKey[1] === novelId),
      });
    },
  });
}

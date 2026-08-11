import { useMutation, useQueryClient } from "@tanstack/react-query";
import { DeleteSkill } from "@/lib/wailsjs/go/app/App";
import { skillKeys } from "@/lib/queryKeys";

// useDeleteSkill: 删除技能 mutation。
// mutationFn 直接 import wailsjs DeleteSkill（不用 useApp）。
// 入参 {name, source}，novelId 从 hook 闭包拼入 app.DeleteSkillInput。
// onSuccess 失效 skillKeys.list(novelId)：列表自动刷新（替代 confirmDelete 内手动 invalidate）。
// mutation 不挂 onError；调用方 try/catch + toastError（对齐 character/style 模式）。
export function useDeleteSkill(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; source: string }) =>
      DeleteSkill({ novel_id: novelId, ...input }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: skillKeys.list(novelId) });
    },
  });
}

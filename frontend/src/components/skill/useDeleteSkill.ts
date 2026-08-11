import { useMutation, useQueryClient } from "@tanstack/react-query";
import { DeleteSkill } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { skillKeys } from "@/lib/queryKeys";

// useDeleteSkill: 删除技能 mutation。
// mutationFn 直接 import wailsjs DeleteSkill（不用 useApp）。
// 入参 app.DeleteSkillInput（后端 Pattern 2：novel_id 是结构体字段，非独立函数参数），
// 调用方传完整结构体 {novel_id, name, source}，hook 原样转发，不闭包拼 novel_id。
// 顺应后端 API 设计（与 useInstallRemoteSkill 模式一致），避免人为造 partial 类型。
// onSuccess 失效 skillKeys.list(novelId)：列表自动刷新（替代 confirmDelete 内手动 invalidate）。
// mutation 不挂 onError；调用方 try/catch + toastError（对齐 character/style 模式）。
export function useDeleteSkill(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: app.DeleteSkillInput) => DeleteSkill(input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: skillKeys.list(novelId) });
    },
  });
}

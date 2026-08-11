import { useMutation, useQueryClient } from "@tanstack/react-query";
import { SaveUserName } from "@/lib/wailsjs/go/app/App";
import { settingsKeys } from "@/lib/queryKeys";

// useSaveUserName: 保存用户名 mutation。
// mutationFn 直接 import wailsjs SaveUserName（不用 useApp）。
// onSuccess invalidate settingsKeys.all —— useProfileSettings 自动 refetch 拿新
// user_name（settingsKeys.all 与 chat 共享缓存，chat 不读 user_name 不受影响）。
// 错误处理由调用方 try/catch + setNameError（保留组件级 inline 错误展示）。
// 5.7 commit 2。
export function useSaveUserName() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => SaveUserName(name),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: settingsKeys.all });
    },
  });
}

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { SaveGitConfig } from "@/lib/wailsjs/go/app/App";
import { settingsKeys } from "@/lib/queryKeys";

// useSaveGitConfig: 保存 Git 提交作者配置 mutation。
// onSuccess invalidate settingsKeys.all —— GetSettings 返回 git_name/git_email 字段，
// 保存后需让 useSettings refetch 拿新值，避免 GeneralConfigTab 切走再切回时
// useEffect 用旧 settingsQuery.data 回填 gitName/gitEmail 显示旧值。
// chat/profile 的 useSettings/useProfileSettings refetch 后数据一样（不读 git 字段），无副作用。
// 错误处理由调用方 try/catch + inline gitError（保留组件级 inline 错误展示）。
// 5.8 commit 2。
export function useSaveGitConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { gitName: string; gitEmail: string }) =>
      SaveGitConfig(input.gitName, input.gitEmail),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: settingsKeys.all });
    },
  });
}

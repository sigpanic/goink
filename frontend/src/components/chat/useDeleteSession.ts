import { useMutation, useQueryClient } from "@tanstack/react-query";
import { DeleteSession } from "@/lib/wailsjs/go/app/App";

// useDeleteSession: 会话删除 mutation。
// onSuccess invalidate ["sessions"] 全前缀（含 list + infiniteList），自动刷新所有会话列表缓存。
// 不挂 onError —— 调用方 try/catch + toastError（对齐规范：mutation 不挂 onError）。
export function useDeleteSession() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => DeleteSession(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["sessions"] });
    },
  });
}

import { useMutation } from "@tanstack/react-query";
import { CancelChat } from "@/lib/wailsjs/go/app/App";

// useCancelChat: 取消进行中的 chat 流式响应 mutation。
// CancelChat 后端仅 cancel context + 清理 cancelMgr map，不写任何持久化数据，
// 故无需 invalidate settings（与 setter mutation 不同，后者确改 settings 字段）。
// 错误处理由调用方 try/catch + toastError（保留组件级 toast）。
// 5.1 commit 4 补遗。
export function useCancelChat() {
  return useMutation({
    mutationFn: (sessionId: string) => CancelChat(sessionId),
  });
}

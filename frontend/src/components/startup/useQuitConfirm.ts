import { useCallback, useEffect, useState } from "react";
import { CancelQuit, ConfirmQuit } from "@/lib/wailsjs/go/app/App";
import { EventsOn } from "@/lib/wailsjs/runtime/runtime";

export interface QuitConfirmStatus {
  /** 后端请求确认退出，应显示关窗确认弹窗。 */
  open: boolean;
  /** 用户确认退出：通知后端关闭窗口。 */
  confirm: () => void;
  /** 用户选择继续等待：清除后端待确认标记，下次关窗重新劝阻。 */
  cancel: () => void;
}

/**
 * 订阅后端的关窗确认请求，把"弹窗 + 回传用户选择"关在 hook 里。
 *
 * 确认由后端发起：OnBeforeClose 在初始化期间拦截关闭并发出 app:quit-confirm，
 * 因为 Wails v2 的 MessageDialog 在 Windows/Linux 会忽略自定义按钮，
 * 无法表达"继续等待 / 退出"这组选项。
 *
 * 两个选择都必须回传后端，它靠标记决定下次关窗是再劝一次还是直接放行：
 * 只关掉弹窗而不取消，用户下一次关窗会被当成"确认期间再次关闭"而不再劝阻。
 */
export function useQuitConfirm(): QuitConfirmStatus {
  const [open, setOpen] = useState(false);

  useEffect(() => {
    const unsubscribe = EventsOn("app:quit-confirm", () => setOpen(true));
    return () => unsubscribe();
  }, []);

  const confirm = useCallback(() => {
    setOpen(false);
    void ConfirmQuit();
  }, []);

  const cancel = useCallback(() => {
    setOpen(false);
    void CancelQuit();
  }, []);

  return { open, confirm, cancel };
}

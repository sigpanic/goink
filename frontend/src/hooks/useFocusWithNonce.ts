import { useEffect } from "react";
import { useFocusStore } from "@/stores/useFocusStore";
import type { PanelId } from "@/types/panel";

// useFocusWithNonce: 封装 focusStore 订阅 + unmount cleanup。
// 返回当前面板的 focus entry（{ id, nonce } | undefined）。
// View unmount 时自动 clearFocus(panelId)，避免切走再切回 remount 时
// focusId 残留触发不期望的重新定位。
// 各 View 的定位逻辑（scrollIntoView / setExpandedId / setWindowCenter 等）
// 在自己的 useEffect 里写，依赖加 focus?.nonce 让「重新点同一条」也触发。
export function useFocusWithNonce(panelId: PanelId) {
  const focus = useFocusStore((s) => s.focusMap[panelId]);
  const clear = useFocusStore((s) => s.clearFocus);
  useEffect(() => {
    return () => clear(panelId);
  }, [panelId, clear]);
  return focus;
}

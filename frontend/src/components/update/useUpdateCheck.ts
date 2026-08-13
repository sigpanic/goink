import { useEffect, useState } from "react";
import { CheckUpdate } from "@/lib/wailsjs/go/app/App";
import type { update } from "@/lib/wailsjs/go/models";

// 自动更新检查的周期：启动后 30s 首次查，之后周期复查。
// 后端 CheckUpdate(false) 另有 12h 节流兜底（跨启动去重 + 防 HMR/StrictMode 重复打）。
const UPDATE_FIRST_DELAY_MS = 30_000;
// 轮询间隔故意略大于后端 12h 节流：首次查在启动 30s 后写 last_check，之后 setInterval 周期触发；
// 若精确等于 12h，首次周期距首次查仅 11h59m30s 会被后端挡掉（首次复查延到 24h）。
// 留 5min 余量既消除 30s 起点差，又吸收 setInterval timer 抖动。产品上 12h vs 12h5m 无感知差异。
const UPDATE_INTERVAL_MS = (12 * 60 + 5) * 60 * 1000; // 12h5m

/**
 * useUpdateCheck 封装自动更新检查的定时调度与结果状态。
 *
 * - 启动后 30s 首次检查，之后每 12h 复查一次；
 * - 仅当检查到新版本时写入 updateResult 并打开 dialog；
 * - 组件卸载时清理定时器，并通过 cancelled 标志避免卸载后 setState。
 *
 * 后端 CheckUpdate(false) 负责 12h 节流（跨启动去重），前端只负责周期触发。
 */
export function useUpdateCheck() {
  const [showUpdate, setShowUpdate] = useState(false);
  const [updateResult, setUpdateResult] = useState<update.CheckResult | null>(
    null,
  );

  useEffect(() => {
    let cancelled = false;
    const doCheck = async () => {
      if (cancelled) return;
      try {
        const result = await CheckUpdate(false);
        if (cancelled) return;
        if (result && result.hasUpdate) {
          setUpdateResult(result);
          setShowUpdate(true);
        }
      } catch {
        /* 静默失败 */
      }
    };
    const firstTimer = setTimeout(doCheck, UPDATE_FIRST_DELAY_MS);
    const interval = setInterval(doCheck, UPDATE_INTERVAL_MS);
    return () => {
      cancelled = true;
      clearTimeout(firstTimer);
      clearInterval(interval);
    };
  }, []);

  return { updateResult, showUpdate, setShowUpdate };
}

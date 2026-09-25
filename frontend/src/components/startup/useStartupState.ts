import { useCallback, useEffect, useRef, useState } from "react";
import { FrontendReady, RetryStartup } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { EventsOn } from "@/lib/wailsjs/runtime/runtime";

/** 迁移遮罩的防抖窗口：状态保持超过它才显示遮罩，避免快启动时闪一下。 */
const PREPARING_DELAY_MS = 500;

export interface StartupStatus {
  /** 当前启动阶段；握手失败时为 "failed"，还没拿到任何快照时为 null。 */
  phase: string | null;
  /** 当前快照版本；null 表示还没拿到快照。 */
  version: number | null;
  /** 当前该展示的启动错误，无错时为空串。 */
  error: string;
  /** 迁移中且已超过防抖窗口，应显示"正在准备数据"遮罩。 */
  showPreparing: boolean;
  /** 本次启动是否由首次配置页点火，用于进入主界面后展示引导。 */
  cameFromInit: boolean;
  /** 失败页的"重试"按钮。 */
  retry: () => Promise<void>;
  retrying: boolean;
}

/**
 * 订阅后端启动状态，把"四态 + 版本号 + 防抖 + 重试"这一整套启动协议关在 hook 里。
 *
 * 后端会用 startup:state 事件推送状态，但事件是瞬时且可能乱序的：
 * ① 快照走两条独立通道（event 推送 / FrontendReady 的返回值），二者顺序不保证；
 * ② 前端先注册再调用 FrontendReady，此时返回的返回值与随后到达的事件也可能交错。
 * 因此这里不比转速，只按 Version 单调递增取舍——旧快照一律丢弃。
 */
export function useStartupState(): StartupStatus {
  const [state, setState] = useState<app.StartupState | null>(null);
  const [bootstrapError, setBootstrapError] = useState("");
  const [retryError, setRetryError] = useState("");
  const [retrying, setRetrying] = useState(false);
  const [cameFromInit, setCameFromInit] = useState(false);
  const [preparingVisibleVersion, setPreparingVisibleVersion] = useState<
    number | null
  >(null);
  const versionRef = useRef(-1);
  const phaseRef = useRef<string | null>(null);

  useEffect(() => {
    let active = true;
    const applyState = (next: app.StartupState) => {
      if (!active || next.version <= versionRef.current) return;

      const previousPhase = phaseRef.current;
      versionRef.current = next.version;
      phaseRef.current = next.phase;
      if (previousPhase === "unconfigured" && next.phase === "initializing") {
        setCameFromInit(true);
      }
      setBootstrapError("");
      setRetryError("");
      setState(next);
    };

    const unsubscribe = EventsOn("startup:state", (data: app.StartupState) => {
      applyState(data);
    });
    // 顺序不能反：先注册监听，再告知后端。FrontendReady 返回的是"这一刻"的快照，
    // 它之后的变化都能通过事件到达；之前的变化已经包含在这份返回值里。
    FrontendReady()
      .then(applyState)
      .catch((error) => {
        if (active) setBootstrapError(String(error));
      });

    return () => {
      active = false;
      unsubscribe();
    };
  }, []);

  useEffect(() => {
    if (state?.phase !== "initializing") return;

    const version = state.version;
    const timer = window.setTimeout(
      () => setPreparingVisibleVersion(version),
      PREPARING_DELAY_MS,
    );
    return () => window.clearTimeout(timer);
  }, [state?.phase, state?.version]);

  const retry = useCallback(async () => {
    setRetryError("");
    setRetrying(true);
    try {
      await RetryStartup();
    } catch (error) {
      setRetryError(String(error));
    } finally {
      setRetrying(false);
    }
  }, []);

  // 握手失败（拿不到任何快照）也归一到 failed，让调用方只需看 phase 一个字段。
  const phase = bootstrapError ? "failed" : state?.phase ?? null;
  const error = bootstrapError || retryError || state?.error || "";
  const showPreparing =
    state?.phase === "initializing" && preparingVisibleVersion === state.version;

  return {
    phase,
    version: state?.version ?? null,
    error,
    showPreparing,
    cameFromInit,
    retry,
    retrying,
  };
}

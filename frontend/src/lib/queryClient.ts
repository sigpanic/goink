import { QueryClient } from "@tanstack/react-query";
import { installQueryErrorToast } from "./queryErrorToast";

// staleTime 30s：避免切面板时短时间重复 fetch。
// 项目无 URL，queryKey 全靠人工设计，staleTime 兜底防抖。
// refetchOnWindowFocus: false —— 桌面应用切窗不应触发重 fetch。
// retry: 1 —— 后端偶尔抖动重试一次，避免无限重试。
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
});

// 4a: 挂载全局 query 错误 toast 中间件。
// 模块顶层挂载一次，避免 StrictMode 双调用。设计详见 docs/feat/v1.4.0/refactor-plan/04a-query-error-toast.md。
installQueryErrorToast(queryClient);

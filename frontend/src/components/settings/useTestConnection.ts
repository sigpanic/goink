import { useMutation } from "@tanstack/react-query";
import { TestConnection } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";

// useTestConnection: 测试 LLM 服务商连通性 mutation。
// 后端多层 fallback 真测，返回验证通过的实际 URL（可能和入参不同）。
// onSuccess 无需 invalidate（命令操作，不改变缓存数据；URL 回写由调用方 setProviders 处理）。
// 错误处理由调用方 try/catch + inline testResults.msg（保留组件级 inline 错误展示）。
// 5.8 commit 1。
export function useTestConnection() {
  return useMutation({
    mutationFn: (input: app.TestConnectionInput) => TestConnection(input),
  });
}

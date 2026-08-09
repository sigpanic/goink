import { useMutation } from "@tanstack/react-query";
import { CompressContext } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";

// useCompressContext: 压缩上下文 mutation。
// 不挂 onSuccess/onError —— 回填 turnId + toastError + isCompressing/compressingRef
// 由调用方 handleCompress 处理（压缩中 turn 动画 + loading 状态在组件内管理）。
export function useCompressContext() {
  return useMutation({
    mutationFn: (input: app.CompressInput) => CompressContext(input),
  });
}

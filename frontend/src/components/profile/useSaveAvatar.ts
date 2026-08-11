import { useMutation } from "@tanstack/react-query";
import { SaveAvatar } from "@/lib/wailsjs/go/app/App";

// useSaveAvatar: 保存头像文件 mutation。
// mutationFn 直接 import wailsjs SaveAvatar（不用 useApp）。
// 无 onSuccess invalidate——头像是一份文件（非 query 缓存数据），靠调用方
// setAvatarKey(prev => prev + 1) 破坏 <img key> 强刷显示（机制保留）。
// 错误处理由调用方 try/catch + setAvatarError（保留组件级 inline 错误展示）。
// 5.7 commit 2。
export function useSaveAvatar() {
  return useMutation({
    mutationFn: (bytes: number[]) => SaveAvatar(bytes),
  });
}

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { SaveCover } from "@/lib/wailsjs/go/app/App";
import { novelKeys } from "@/lib/queryKeys";

// useSaveCover: 保存小说封面 mutation。
// mutationFn 直接 import wailsjs SaveCover（不用 useApp），onSuccess 失效 novelKeys.all
// 让 useNovels refetch 拿新封面 URL。
// 错误处理由调用方 try/catch + toastError（novel.coverSaveFailed）。
// 入参 {novelId, cover} 解构：novelId 是小说 ID，cover 是封面字节数组（Array<number>）。
export function useSaveCover() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ novelId, cover }: { novelId: number; cover: number[] }) =>
      SaveCover(novelId, cover),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: novelKeys.all });
    },
  });
}

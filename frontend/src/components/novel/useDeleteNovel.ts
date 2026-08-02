import { useMutation, useQueryClient } from "@tanstack/react-query";
import { DeleteNovel } from "@/lib/wailsjs/go/app/App";
import { novelKeys } from "@/lib/queryKeys";

// useDeleteNovel: 删除小说 mutation。
// mutationFn 直接 import wailsjs DeleteNovel（不用 useApp），onSuccess 失效 novelKeys.all。
// handler 负责 setDeletingNovel(null) + 错误 throw（副作用各异，不放进 mutation）。
// 删除当前 activeNovelId 时，refetch 后由自动选小说 effect（WorkspaceView）选第一个接管。
export function useDeleteNovel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => DeleteNovel(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: novelKeys.all });
    },
  });
}

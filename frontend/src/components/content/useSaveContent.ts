import { useMutation, useQueryClient } from "@tanstack/react-query";
import { SaveContent } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { contentKeys } from "@/lib/queryKeys";

// useSaveContent: 保存文件内容 mutation。
// mutationFn 直接 import wailsjs SaveContent（不用 useApp），单参 input（含 novel_id + path + content）。
// onSuccess 失效 contentKeys.detail(input.novel_id, input.path)：保持 useFileContent 缓存一致性，
// 外部读取同 path 时不会拿到旧 content（如 file:changed handler、多 tab 同 path 场景）。
// 调用方 doSave 负责 updateTab(isDirty:false) + try/catch + toastError（tab 是本地 state，不进 query cache）。
//
// 5.3 复用：StyleView/PatternSessionView 保存 skill/style 内容走此 hook（通用，不绑死 chapter 语义）。
export function useSaveContent() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: app.SaveContentInput) => SaveContent(input),
    onSuccess: (_data, input) => {
      qc.invalidateQueries({
        queryKey: contentKeys.detail(input.novel_id, input.path),
      });
    },
  });
}

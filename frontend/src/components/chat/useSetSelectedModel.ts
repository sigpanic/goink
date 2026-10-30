import { useMutation, useQueryClient } from "@tanstack/react-query";
import { SetSelectedModel } from "@/lib/wailsjs/go/app/App";
import { app, config } from "@/lib/wailsjs/go/models";
import { sessionKeys, settingsKeys } from "@/lib/queryKeys";

// useSetSelectedModel 保存完整模型选择（协议、服务商实例、模型及推理等级）。
// session_id 为空时更新全局默认值，否则只更新指定会话。
// 同一 mutation scope 串行保存；成功后更新并刷新对应的 settings 或 session 缓存。
// 错误由 useChatModelSelection 保留用户草稿并提示。
export function useSetSelectedModel() {
  const qc = useQueryClient();
  return useMutation({
    scope: { id: "chat-model-selection" },
    mutationFn: (input: app.SetSelectedModelInput) => SetSelectedModel(input),
    onSuccess: async (_, input) => {
      const queryKey = input.session_id
        ? sessionKeys.detail(input.session_id)
        : settingsKeys.all;
      await qc.cancelQueries({ queryKey, exact: true });
      if (input.session_id) {
        qc.setQueryData<app.SessionDetail>(queryKey, (current) =>
          current
            ? new app.SessionDetail({
                ...current,
                model_selection: input.model_selection,
              })
            : current,
        );
      } else {
        qc.setQueryData<config.AppSettings>(queryKey, (current) =>
          current
            ? new config.AppSettings({
                ...current,
                default_model_selection: input.model_selection,
              })
            : current,
        );
      }
      await qc.invalidateQueries({ queryKey, exact: true });
    },
  });
}

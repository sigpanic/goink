import { useQuery } from "@tanstack/react-query";
import { GetSettings } from "@/lib/wailsjs/go/app/App";
import { settingsKeys } from "@/lib/queryKeys";

// useSettings: 应用设置 query（含 selected_model_key / reasoning_effort /
// approval_mode / last_session_id 等持久化字段）。
// queryFn 直接 import wailsjs GetSettings（不用 useApp）。
// ChatPanel 选中态恢复依赖本 query data + useModels data 都 ready。
export function useSettings() {
  return useQuery({
    queryKey: settingsKeys.all,
    queryFn: async () => {
      const settings = await GetSettings();
      return settings;
    },
  });
}

import { useQuery } from "@tanstack/react-query";
import { ListSlashCommands } from "@/lib/wailsjs/go/app/App";
import { slashCommandKeys } from "@/lib/queryKeys";

// useSlashCommands: 斜杠命令列表 query（ChatInput 的 SlashMenu 消费）。
// enabled: !!novelId 守卫，novelId=0 时不 fetch。
export function useSlashCommands(novelId: number) {
  return useQuery({
    queryKey: slashCommandKeys.list(novelId),
    queryFn: async () => {
      const list = await ListSlashCommands({ novel_id: novelId });
      return list ?? [];
    },
    enabled: !!novelId,
  });
}

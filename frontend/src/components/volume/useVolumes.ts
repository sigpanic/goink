import { useQuery } from "@tanstack/react-query";
import { GetVolumes } from "@/lib/wailsjs/go/app/App";
import { volumeKeys } from "@/lib/queryKeys";

export function useVolumes(novelId: number) {
  return useQuery({
    queryKey: volumeKeys.list(novelId),
    queryFn: async () => (await GetVolumes(novelId)) ?? [],
    enabled: !!novelId,
  });
}

import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  DeleteVolume,
  PlaceVolume,
  UpdateVolume,
} from "@/lib/wailsjs/go/app/App";
import type { volume } from "@/lib/wailsjs/go/models";
import { chapterKeys, volumeKeys } from "@/lib/queryKeys";

export function useVolumeMutations(novelId: number) {
  const qc = useQueryClient();
  const refresh = async () => {
    await Promise.all([
      qc.invalidateQueries({ queryKey: volumeKeys.list(novelId) }),
      qc.invalidateQueries({ queryKey: chapterKeys.list(novelId) }),
    ]);
  };

  const place = useMutation({
    mutationFn: (input: volume.PlaceInput) => PlaceVolume(input),
    onSuccess: refresh,
  });
  const rename = useMutation({
    mutationFn: ({ id, name }: { id: number; name: string }) =>
      UpdateVolume(novelId, id, name),
    onSuccess: refresh,
  });
  const remove = useMutation({
    mutationFn: (id: number) => DeleteVolume(novelId, id),
    onSuccess: refresh,
  });

  return { place, rename, remove };
}

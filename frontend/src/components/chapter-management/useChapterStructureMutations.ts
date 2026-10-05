import { useMutation, useQueryClient } from "@tanstack/react-query";
import { DeleteChapter, PlaceChapter } from "@/lib/wailsjs/go/app/App";
import type { chapter } from "@/lib/wailsjs/go/models";
import { chapterKeys, maxChapterKeys } from "@/lib/queryKeys";

export function useChapterStructureMutations(novelId: number) {
  const qc = useQueryClient();
  const refresh = async () => {
    await Promise.all([
      qc.invalidateQueries({ queryKey: chapterKeys.list(novelId) }),
      qc.invalidateQueries({ queryKey: maxChapterKeys.detail(novelId) }),
    ]);
  };

  const place = useMutation({
    mutationFn: (input: chapter.PlaceInput) => PlaceChapter(input),
    onSuccess: refresh,
  });
  const remove = useMutation({
    mutationFn: (id: number) => DeleteChapter(novelId, id),
    onSuccess: async (result) => {
      if (result.deleted) await refresh();
    },
  });

  return { place, remove };
}

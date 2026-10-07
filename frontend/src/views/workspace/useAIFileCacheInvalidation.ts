import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { EventsOn } from "@/lib/wailsjs/runtime/runtime";
import {
  chapterKeys,
  contentKeys,
  maxChapterKeys,
  skillKeys,
} from "@/lib/queryKeys";
import { reportAIFileChange } from "@/components/content/aiFileChanges";

interface FileChangedEvent {
  novel_id?: number;
  path?: string;
  metadata_only?: boolean;
}

export function useAIFileCacheInvalidation() {
  const qc = useQueryClient();

  useEffect(() => {
    return EventsOn("file:changed", (data: FileChangedEvent) => {
      const novelId = data?.novel_id;
      const filePath = data?.path;
      if (
        typeof novelId !== "number" ||
        !Number.isSafeInteger(novelId) ||
        novelId < 1 ||
        !filePath
      )
        return;

      if (data.metadata_only === true) {
        if (
          filePath.startsWith("chapters/") ||
          filePath.startsWith("outlines/")
        ) {
          void qc.invalidateQueries({ queryKey: chapterKeys.list(novelId) });
        }
        return;
      }

      if (filePath.startsWith("~/.goink/skills/")) {
        const filter = {
          predicate: ({ queryKey }: { queryKey: readonly unknown[] }) =>
            queryKey[0] === contentKeys.all[0] && queryKey[2] === filePath,
        };
        void qc.cancelQueries(filter);
        void qc.invalidateQueries({
          predicate: filter.predicate,
        });
      } else {
        void qc.cancelQueries({
          queryKey: contentKeys.detail(novelId, filePath),
        });
        void qc.invalidateQueries({
          queryKey: contentKeys.detail(novelId, filePath),
        });
      }

      if (
        filePath.startsWith("chapters/") ||
        filePath.startsWith("outlines/") ||
        filePath === "goink.md"
      ) {
        void qc.invalidateQueries({ queryKey: chapterKeys.list(novelId) });
      }
      if (filePath.startsWith("chapters/")) {
        void qc.invalidateQueries({
          queryKey: maxChapterKeys.detail(novelId),
        });
      }
      if (filePath.startsWith("skills/")) {
        void qc.invalidateQueries({ queryKey: skillKeys.list(novelId) });
      } else if (filePath.startsWith("~/.goink/skills/")) {
        void qc.invalidateQueries({ queryKey: skillKeys.all });
      }

      reportAIFileChange({ novelId, path: filePath });
    });
  }, [qc]);
}

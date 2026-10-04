import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { EventsOn } from "@/lib/wailsjs/runtime/runtime";
import {
  chapterKeys,
  contentKeys,
  maxChapterKeys,
  skillKeys,
} from "@/lib/queryKeys";

interface FileChangedEvent {
  novel_id?: number;
  path?: string;
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

      if (filePath.startsWith("~/.goink/skills/")) {
        void qc.invalidateQueries({
          predicate: ({ queryKey }) =>
            queryKey[0] === contentKeys.all[0] && queryKey[2] === filePath,
        });
      } else {
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
    });
  }, [qc]);
}

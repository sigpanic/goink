import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { GetChapters, GetVolumes } from "@/lib/wailsjs/go/app/App";
import { chapterKeys, volumeKeys } from "@/lib/queryKeys";
import { useEditorTabsStore } from "./useEditorTabsStore";
import { describeFile } from "./fileDescription";
import { isOutlinePath, isVolumeOutlinePath, type EditorTab } from "./types";

const EMPTY_TABS: EditorTab[] = [];

export function useEditorTabTitles(novelId: number) {
  const { t } = useTranslation();
  const tabs = useEditorTabsStore(
    (s) => s.byNovel[String(novelId)]?.tabs ?? EMPTY_TABS,
  );
  const chapters = useQuery({
    queryKey: chapterKeys.list(novelId),
    queryFn: async () => (await GetChapters(novelId)) ?? [],
    enabled:
      novelId > 0 &&
      tabs.some(
        (tab) => tab.path.startsWith("chapters/") || isOutlinePath(tab.path),
      ),
  });
  const volumes = useQuery({
    queryKey: volumeKeys.list(novelId),
    queryFn: async () => (await GetVolumes(novelId)) ?? [],
    enabled: novelId > 0 && tabs.some((tab) => isVolumeOutlinePath(tab.path)),
  });
  useEffect(() => {
    const store = useEditorTabsStore.getState();
    for (const tab of store.byNovel[String(novelId)]?.tabs ?? []) {
      const description = describeFile(
        tab.path,
        t,
        chapters.data,
        volumes.data,
      );
      const title =
        tab.type === "diff" ? description.diffTitle : description.title;
      if (description.resolved && title !== tab.title)
        store.updateTab(novelId, tab.id, { title });
    }
  }, [novelId, tabs, chapters.data, volumes.data, t]);
}

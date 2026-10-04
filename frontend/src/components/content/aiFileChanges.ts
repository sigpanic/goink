import { useEditorTabsStore } from "./useEditorTabsStore";

export interface AIFileChange {
  novelId: number;
  path: string;
}

const versions = new Map<string, number>();
const listeners = new Set<(change: AIFileChange) => void>();

function key(novelId: number, path: string): string {
  return path.startsWith("~/.goink/skills/")
    ? `user:${path}`
    : `${novelId}:${path}`;
}

export function aiFileVersion(novelId: number, path: string): number {
  return versions.get(key(novelId, path)) ?? 0;
}

export function subscribeAIFileChanges(
  listener: (change: AIFileChange) => void,
): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function reportAIFileChange(change: AIFileChange): void {
  const { novelId, path } = change;
  const fileKey = key(novelId, path);
  versions.set(fileKey, (versions.get(fileKey) ?? 0) + 1);

  const tabsStore = useEditorTabsStore.getState();
  for (const [novelKey, entry] of Object.entries(tabsStore.byNovel)) {
    const tabNovelId = Number(novelKey);
    if (
      tabNovelId !== novelId &&
      !path.startsWith("~/.goink/skills/")
    )
      continue;
    for (const tab of entry.tabs) {
      if (tab.type !== "file") continue;
      if (tab.path === path) {
        tabsStore.updateTab(tabNovelId, tab.id, { contentNeedsRefresh: true });
      } else if (tab.outlinePath === path) {
        tabsStore.updateTab(tabNovelId, tab.id, {
          outlineNeedsRefresh: true,
        });
      }
    }
  }

  for (const listener of listeners) listener(change);
}

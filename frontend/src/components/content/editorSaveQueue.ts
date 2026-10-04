import type { QueryClient } from "@tanstack/react-query";

export interface EditorSaveDraft {
  novelId: number;
  tabId: string;
  path: string;
  content: string;
  dirtyKey: "isDirty" | "outlineIsDirty";
}

interface PendingDraft extends EditorSaveDraft {
  revision: number;
  save: () => Promise<unknown>;
  onSaved: () => void;
  onError: (error: unknown) => void;
}

interface SaveEntry {
  key: string;
  latest: PendingDraft;
  savedRevision: number;
  timer: ReturnType<typeof setTimeout> | null;
  inFlight: Promise<boolean> | null;
}

const SAVE_DELAY_MS = 500;

function fileKey(novelId: number, path: string): string {
  return `${novelId}:${path}`;
}

export class EditorSaveQueue {
  private entries = new Map<string, SaveEntry>();

  schedule(
    draft: EditorSaveDraft,
    save: () => Promise<unknown>,
    onSaved: () => void,
    onError: (error: unknown) => void,
  ) {
    const key = fileKey(draft.novelId, draft.path);
    const entry = this.entries.get(key);
    const latest: PendingDraft = {
      ...draft,
      revision: (entry?.latest.revision ?? 0) + 1,
      save,
      onSaved,
      onError,
    };
    if (entry) {
      if (entry.timer) clearTimeout(entry.timer);
      entry.latest = latest;
      entry.timer = setTimeout(() => {
        entry.timer = null;
        void this.write(entry).catch(() => undefined);
      }, SAVE_DELAY_MS);
      return;
    }

    const next: SaveEntry = {
      key,
      latest,
      savedRevision: 0,
      timer: null,
      inFlight: null,
    };
    next.timer = setTimeout(() => {
      next.timer = null;
      void this.write(next).catch(() => undefined);
    }, SAVE_DELAY_MS);
    this.entries.set(key, next);
  }

  async flush(novelId: number, path: string): Promise<boolean> {
    const entry = this.entries.get(fileKey(novelId, path));
    if (!entry) return false;

    while (entry.savedRevision < entry.latest.revision) {
      if (entry.timer) {
        clearTimeout(entry.timer);
        entry.timer = null;
      }
      if (!(await (entry.inFlight ?? this.write(entry)))) return false;
    }
    return true;
  }

  private write(entry: SaveEntry): Promise<boolean> {
    if (entry.inFlight) return entry.inFlight;
    const draft = entry.latest;
    let succeeded = false;
    const task = (async () => {
      try {
        await draft.save();
        succeeded = true;
        entry.savedRevision = draft.revision;
        if (entry.latest.revision === draft.revision) draft.onSaved();
        return true;
      } catch (error) {
        draft.onError(error);
        return false;
      } finally {
        entry.inFlight = null;
        if (
          succeeded &&
          entry.latest.revision > draft.revision &&
          !entry.timer
        ) {
          void this.write(entry);
        } else if (
          succeeded &&
          entry.latest.revision === draft.revision &&
          this.entries.get(entry.key) === entry
        ) {
          this.entries.delete(entry.key);
        }
      }
    })();
    entry.inFlight = task;
    return task;
  }
}

const queues = new WeakMap<QueryClient, EditorSaveQueue>();

export function getEditorSaveQueue(qc: QueryClient): EditorSaveQueue {
  let queue = queues.get(qc);
  if (!queue) {
    queue = new EditorSaveQueue();
    queues.set(qc, queue);
  }
  return queue;
}

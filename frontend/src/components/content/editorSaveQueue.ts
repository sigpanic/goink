import type { QueryClient } from "@tanstack/react-query";

export interface EditorSaveDraft {
  novelId: number;
  tabId: string;
  path: string;
  content: string;
  dirtyKey: "isDirty" | "outlineIsDirty";
  expectedContent: string;
}

interface PendingDraft extends EditorSaveDraft {
  revision: number;
  save: (expectedContent: string) => Promise<unknown>;
  onWritten: (content: string) => void;
  onSaved: () => void;
  onError: (error: unknown) => void;
}

interface SaveEntry {
  key: string;
  latest: PendingDraft;
  savedRevision: number;
  timer: ReturnType<typeof setTimeout> | null;
  inFlight: Promise<boolean> | null;
  baseContent: string;
  paused: boolean;
}

const SAVE_DELAY_MS = 500;

export function isContentConflict(error: unknown): boolean {
  return String(error).includes("CONTENT_CONFLICT:");
}

function fileKey(novelId: number, path: string): string {
  return `${novelId}:${path}`;
}

export class EditorSaveQueue {
  private entries = new Map<string, SaveEntry>();

  schedule(
    draft: EditorSaveDraft,
    save: (expectedContent: string) => Promise<unknown>,
    onWritten: (content: string) => void,
    onSaved: () => void,
    onError: (error: unknown) => void,
  ) {
    const key = fileKey(draft.novelId, draft.path);
    const entry = this.entries.get(key);
    const latest: PendingDraft = {
      ...draft,
      revision: (entry?.latest.revision ?? 0) + 1,
      save,
      onWritten,
      onSaved,
      onError,
    };
    if (entry) {
      if (entry.timer) clearTimeout(entry.timer);
      entry.latest = latest;
      if (!entry.paused) this.startTimer(entry);
      return;
    }

    const next: SaveEntry = {
      key,
      latest,
      savedRevision: 0,
      timer: null,
      inFlight: null,
      baseContent: draft.expectedContent,
      paused: false,
    };
    this.startTimer(next);
    this.entries.set(key, next);
  }

  private startTimer(entry: SaveEntry) {
    entry.timer = setTimeout(() => {
      entry.timer = null;
      void this.write(entry).catch(() => undefined);
    }, SAVE_DELAY_MS);
  }

  pause(novelId: number, path: string): void {
    const entry = this.entries.get(fileKey(novelId, path));
    if (!entry) return;
    entry.paused = true;
    if (entry.timer) clearTimeout(entry.timer);
    entry.timer = null;
  }

  isPending(novelId: number, path: string): boolean {
    return this.entries.has(fileKey(novelId, path));
  }

  async waitForIdle(novelId: number, path: string): Promise<void> {
    await this.entries.get(fileKey(novelId, path))?.inFlight;
  }

  discard(novelId: number, path: string): void {
    const key = fileKey(novelId, path);
    const entry = this.entries.get(key);
    if (entry?.inFlight) throw new Error("save still in flight");
    if (entry?.timer) clearTimeout(entry.timer);
    this.entries.delete(key);
  }

  async flush(novelId: number, path: string): Promise<boolean> {
    const entry = this.entries.get(fileKey(novelId, path));
    if (!entry || entry.paused) return false;

    while (entry.savedRevision < entry.latest.revision) {
      if (entry.timer) {
        clearTimeout(entry.timer);
        entry.timer = null;
      }
      if (!(await (entry.inFlight ?? this.write(entry)))) return false;
      if (entry.paused) return false;
    }
    return true;
  }

  private write(entry: SaveEntry): Promise<boolean> {
    if (entry.inFlight) return entry.inFlight;
    if (entry.paused) return Promise.resolve(false);
    const draft = entry.latest;
    const expectedContent = entry.baseContent;
    let succeeded = false;
    const task = (async () => {
      try {
        await draft.save(expectedContent);
        succeeded = true;
        entry.baseContent = draft.content;
        entry.savedRevision = draft.revision;
        draft.onWritten(draft.content);
        if (!entry.paused && entry.latest.revision === draft.revision)
          draft.onSaved();
        return true;
      } catch (error) {
        if (isContentConflict(error)) this.pause(draft.novelId, draft.path);
        draft.onError(error);
        return false;
      } finally {
        entry.inFlight = null;
        if (
          succeeded &&
          entry.latest.revision > draft.revision &&
          !entry.paused &&
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

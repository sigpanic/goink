import { useEffect, useRef, type FormEvent, type KeyboardEvent } from "react";
import { createPortal } from "react-dom";
import type { chapter, volume } from "@/lib/wailsjs/go/models";
import ChapterEditorForm, { type ChapterEditor } from "./ChapterEditorForm";

interface Props {
  editor: ChapterEditor;
  volumes: volume.Volume[];
  targetItems: chapter.Chapter[];
  busy: boolean;
  showPosition: boolean;
  onChange: (editor: ChapterEditor) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onClose: () => void;
}

export default function ChapterCreateDialog({
  editor,
  volumes,
  targetItems,
  busy,
  showPosition,
  onChange,
  onSubmit,
  onClose,
}: Props) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const returnFocusRef = useRef<HTMLElement | null>(
    document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null,
  );

  useEffect(() => {
    const returnFocus = returnFocusRef.current;
    return () => {
      if (returnFocus?.isConnected) {
        returnFocus.focus();
      }
    };
  }, []);

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === "Escape") {
      if (dialogRef.current?.querySelector('[role="listbox"]')) return;
      event.preventDefault();
      if (!busy) onClose();
      return;
    }
    if (event.key !== "Tab") return;
    const focusable = dialogRef.current?.querySelectorAll<HTMLElement>(
      'input:not(:disabled), button:not(:disabled), [tabindex]:not([tabindex="-1"])',
    );
    if (!focusable?.length) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div
        aria-hidden="true"
        className="absolute inset-0 bg-black/40"
        onMouseDown={() => {
          if (!busy) onClose();
        }}
      />
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="chapter-create-dialog-title"
        onKeyDown={handleKeyDown}
        className="relative w-full max-w-md rounded-xl border bg-background p-6 shadow-2xl"
      >
        <ChapterEditorForm
          editor={editor}
          volumes={volumes}
          targetItems={targetItems}
          busy={busy}
          dialog
          showPosition={showPosition}
          onChange={onChange}
          onSubmit={onSubmit}
          onClose={onClose}
        />
      </div>
    </div>,
    document.body,
  );
}

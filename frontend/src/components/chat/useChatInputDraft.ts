import { useState, useCallback, useRef, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { ImportChatImage } from "@/lib/wailsjs/go/app/App";
import { app, type attachment } from "@/lib/wailsjs/go/models";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";
import type { SendResult } from "./types";

// 草稿 localStorage key 前缀（per-session 持久化，沿用 goink_ 惯例）
const DRAFT_PREFIX = "goink_chat_draft_";
const IMAGE_DRAFT_PREFIX = "goink_chat_images_";

function loadImageDraft(key: string): attachment.Ref[] {
  try {
    const saved = JSON.parse(
      localStorage.getItem(IMAGE_DRAFT_PREFIX + key) || "[]",
    );
    return Array.isArray(saved) ? saved : [];
  } catch {
    return [];
  }
}

function readImage(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result).split(",")[1]);
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}

interface Options {
  novelId: number;
  draftKey: string;
  disabled: boolean;
  onSend: (message: string, images: attachment.Ref[]) => Promise<SendResult>;
}

export default function useChatInputDraft({
  novelId,
  draftKey,
  disabled,
  onSend,
}: Options) {
  const { t } = useTranslation();
  const [hasContent, setHasContent] = useState(false);
  const [images, setImages] = useState<attachment.Ref[]>([]);
  const imagesRef = useRef<attachment.Ref[]>([]);
  const [isImporting, setIsImporting] = useState(false);
  const importingRef = useRef(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const draftTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastDraftKeyRef = useRef(draftKey);

  const saveImages = useCallback((key: string, next: attachment.Ref[]) => {
    if (next.length)
      localStorage.setItem(IMAGE_DRAFT_PREFIX + key, JSON.stringify(next));
    else localStorage.removeItem(IMAGE_DRAFT_PREFIX + key);
    if (lastDraftKeyRef.current === key) {
      imagesRef.current = next;
      setImages(next);
    }
  }, []);

  const updateImages = useCallback(
    (next: attachment.Ref[]) => saveImages(draftKey, next),
    [draftKey, saveImages],
  );

  const importImages = useCallback(
    async (files: File[]) => {
      if (disabled || importingRef.current) return;
      importingRef.current = true;
      setIsImporting(true);
      const scope = lastDraftKeyRef.current;
      try {
        for (const file of files) {
          if (loadImageDraft(scope).length >= 4) {
            toastError(t("chat.imageLimit"));
            break;
          }
          if (
            file.size > 20 * 1024 * 1024 ||
            !["image/png", "image/jpeg"].includes(file.type)
          ) {
            toastError(t("chat.invalidImage"));
            continue;
          }
          try {
            const image = await ImportChatImage(
              new app.ChatImageInput({
                novel_id: novelId,
                name: file.name,
                data: await readImage(file),
              }),
            );
            saveImages(scope, [
              ...loadImageDraft(scope),
              { attachment_id: image.id, variant: "standard-v1" },
            ]);
          } catch (err) {
            toastError(toErrorMessage(err, t("chat.imageImportFailed")));
          }
        }
      } finally {
        importingRef.current = false;
        setIsImporting(false);
      }
    },
    [disabled, novelId, saveImages, t],
  );

  const submit = useCallback(
    (value: string) => {
      const sentImages = imagesRef.current;
      const scope = lastDraftKeyRef.current;
      saveImages(scope, []);
      const restore = (sessionId: string) => {
        const target = sessionId || scope;
        const isCurrent = lastDraftKeyRef.current === target;
        const currentText = isCurrent
          ? textareaRef.current?.value
          : localStorage.getItem(DRAFT_PREFIX + target);
        const currentImages = isCurrent
          ? imagesRef.current
          : loadImageDraft(target);
        if (currentText || currentImages.length) return;
        localStorage.setItem(DRAFT_PREFIX + target, value);
        saveImages(target, sentImages);
        if (isCurrent && textareaRef.current) {
          textareaRef.current.value = value;
          textareaRef.current.style.height = "auto";
          textareaRef.current.style.height =
            Math.min(textareaRef.current.scrollHeight, 180) + "px";
          setHasContent(value.trim().length > 0);
        }
      };
      void onSend(value, sentImages)
        .then((result) => {
          if (!result.messageSaved) restore(result.sessionId);
        })
        .catch((err) => {
          toastError(toErrorMessage(err, t("chat.chatError")));
        });
      const textarea = textareaRef.current;
      if (textarea) {
        textarea.value = "";
        textarea.style.height = "auto";
      }
      setHasContent(false);
      // 发送成功：清掉已落盘/待写入的草稿
      if (draftTimerRef.current) {
        clearTimeout(draftTimerRef.current);
        draftTimerRef.current = null;
      }
      localStorage.removeItem(DRAFT_PREFIX + lastDraftKeyRef.current);
    },
    [onSend, saveImages, t],
  );

  const updateText = useCallback((target: HTMLTextAreaElement) => {
    target.style.height = "auto";
    target.style.height = Math.min(target.scrollHeight, 180) + "px";
    setHasContent(target.value.trim().length > 0);
    // 草稿持久化：防抖 300ms 落盘（与 useLayoutState 同风格）
    if (draftTimerRef.current) clearTimeout(draftTimerRef.current);
    draftTimerRef.current = setTimeout(() => {
      draftTimerRef.current = null;
      localStorage.setItem(
        DRAFT_PREFIX + lastDraftKeyRef.current,
        target.value,
      );
    }, 300);
  }, []);

  // 草稿恢复：draftKey 变化时先落盘旧 key 的当前内容，再恢复新 key 的草稿。
  useEffect(() => {
    const key = draftKey;
    const prevKey = lastDraftKeyRef.current;
    lastDraftKeyRef.current = key;

    if (draftTimerRef.current) {
      clearTimeout(draftTimerRef.current);
      draftTimerRef.current = null;
    }

    const textarea = textareaRef.current;
    if (prevKey && prevKey !== key) {
      const current = textarea?.value ?? "";
      if (current) localStorage.setItem(DRAFT_PREFIX + prevKey, current);
      else localStorage.removeItem(DRAFT_PREFIX + prevKey);
    }

    const saved = localStorage.getItem(DRAFT_PREFIX + key);
    const savedImages = loadImageDraft(key);
    imagesRef.current = savedImages;
    setImages(savedImages);
    if (textarea) {
      textarea.value = saved ?? "";
      textarea.style.height = "auto";
      textarea.style.height = Math.min(textarea.scrollHeight, 180) + "px";
      setHasContent((saved ?? "").trim().length > 0);
    }
  }, [draftKey]);

  // 草稿兜底：关窗前 flush 一次；卸载时清掉待写入 timer。
  useEffect(() => {
    const flush = () => {
      if (draftTimerRef.current) {
        clearTimeout(draftTimerRef.current);
        draftTimerRef.current = null;
      }
      const textarea = textareaRef.current;
      if (textarea?.value) {
        localStorage.setItem(
          DRAFT_PREFIX + lastDraftKeyRef.current,
          textarea.value,
        );
      }
    };
    window.addEventListener("beforeunload", flush);
    return () => {
      window.removeEventListener("beforeunload", flush);
      if (draftTimerRef.current) {
        clearTimeout(draftTimerRef.current);
        draftTimerRef.current = null;
      }
    };
  }, []);

  return {
    textareaRef,
    hasContent,
    setHasContent,
    images,
    isImporting,
    updateImages,
    importImages,
    updateText,
    submit,
  };
}

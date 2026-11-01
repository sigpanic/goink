import { useState, useCallback, useRef, useEffect, useMemo } from "react";
import { ArrowUp, Square, Zap, Play, Star, Loader2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { app } from "@/lib/wailsjs/go/models";
import ChatImages from "./ChatImages";
import SlashMenu from "./SlashMenu";
import type useChatInputDraft from "./useChatInputDraft";

// charMatch 检查 q 的所有字符是否按顺序出现在 s 中（模糊匹配）
const charMatch = (s: string, q: string): boolean => {
  let qi = 0;
  for (let i = 0; i < s.length && qi < q.length; i++) {
    if (s[i] === q[qi]) qi++;
  }
  return qi === q.length;
};

// score 计算匹配得分，越低越好（0=完全匹配，1=前缀，2=包含，3=字符顺序，4=描述，5=不匹配）
const score = (c: app.SlashCommand, q: string): number => {
  const name = c.name.toLowerCase();
  if (name === q) return 0;
  if (name.startsWith(q)) return 1;
  if (name.includes(q)) return 2;
  if (charMatch(name, q)) return 3;
  if (c.description.toLowerCase().includes(q)) return 4;
  return 5;
};

interface Props {
  novelId: number;
  supportsVision: boolean;
  disabled: boolean;
  isLoading: boolean;
  isCancelling: boolean;
  placeholder: string;
  draft: ReturnType<typeof useChatInputDraft>;
  slashItems: app.SlashCommand[];
  onStop: () => void;
  onListSlash: () => void;
}

export default function ChatInput({
  novelId,
  supportsVision,
  disabled,
  isLoading,
  isCancelling,
  placeholder,
  draft,
  slashItems,
  onStop,
  onListSlash,
}: Props) {
  const { t } = useTranslation();
  const {
    textareaRef,
    hasContent,
    setHasContent,
    images,
    isImporting,
    updateImages,
    importImages,
    updateText,
    submit,
  } = draft;

  // slash menu state
  const [slashOpen, setSlashOpen] = useState(false);
  const [slashFilter, setSlashFilter] = useState("");
  const [slashIndex, setSlashIndex] = useState(0);
  const [slashPos, setSlashPos] = useState({ top: 0, left: 0, width: 0 });
  const [activeCommand, setActiveCommand] = useState<app.SlashCommand | null>(
    null,
  );

  const q = slashFilter.toLowerCase();
  // filteredItems 用 score 过滤+排序，与 SlashMenu 渲染列表一致（修复高亮≠选中 bug）
  const filteredItems = useMemo(() => {
    if (!q) return slashItems;
    return slashItems
      .filter((c) => score(c, q) < 5)
      .sort((a, b) => score(a, q) - score(b, q));
  }, [slashItems, q]);

  // 跟踪上次 filter 值，filter 变化时同步重置 slashIndex（避免 useEffect 异步窗口导致 Enter 取到 undefined）
  const prevFilterRef = useRef("");

  const closeSlash = useCallback(() => {
    setSlashOpen(false);
    setSlashFilter("");
    setSlashIndex(0);
    prevFilterRef.current = "";
  }, []);

  const applySlashSelection = useCallback(
    (cmd: app.SlashCommand) => {
      const textarea = textareaRef.current;
      if (!textarea) return;
      textarea.value = "/" + cmd.name + " ";
      textarea.focus();
      textarea.setSelectionRange(textarea.value.length, textarea.value.length);
      textarea.style.height = "auto";
      textarea.style.height = Math.min(textarea.scrollHeight, 180) + "px";
      setHasContent(true);
      setActiveCommand(cmd);
      closeSlash();
    },
    [closeSlash, setHasContent, textareaRef],
  );

  const updateSlashPos = useCallback(() => {
    const textarea = textareaRef.current;
    if (!textarea) return;
    const rect = textarea.getBoundingClientRect();
    setSlashPos({ top: rect.top, left: rect.left, width: rect.width });
  }, [textareaRef]);

  const checkSlash = useCallback(
    (value: string) => {
      if (value.length > 0 && value[0] === "/") {
        const spaceIdx = value.indexOf(" ");
        if (spaceIdx > 1) {
          const name = value.slice(1, spaceIdx);
          setActiveCommand(slashItems.find((c) => c.name === name) ?? null);
          closeSlash();
          return;
        }
        setActiveCommand(null);
        updateSlashPos();
        const newFilter = value.slice(1);
        if (newFilter !== prevFilterRef.current) {
          setSlashIndex(0);
        }
        prevFilterRef.current = newFilter;
        setSlashFilter(newFilter);
        setSlashOpen(true);
        onListSlash();
      } else {
        setActiveCommand(null);
        closeSlash();
      }
    },
    [closeSlash, updateSlashPos, onListSlash, slashItems],
  );

  // close slash on resize/scroll
  useEffect(() => {
    if (!slashOpen) return;
    const handler = () => updateSlashPos();
    window.addEventListener("resize", handler);
    window.addEventListener("scroll", handler, true);
    return () => {
      window.removeEventListener("resize", handler);
      window.removeEventListener("scroll", handler, true);
    };
  }, [slashOpen, updateSlashPos]);

  const handleSendClick = useCallback(() => {
    const textarea = textareaRef.current;
    if (!textarea) return;
    const value = textarea.value.trim();
    if (
      (value || images.length > 0) &&
      !disabled &&
      !isImporting &&
      (images.length === 0 || supportsVision)
    ) {
      submit(value);
      setActiveCommand(null);
      closeSlash();
    }
  }, [
    textareaRef,
    disabled,
    images.length,
    isImporting,
    supportsVision,
    submit,
    closeSlash,
  ]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (slashOpen && filteredItems.length > 0) {
        if (e.key === "ArrowDown") {
          e.preventDefault();
          setSlashIndex((i) => (i + 1) % filteredItems.length);
          return;
        }
        if (e.key === "ArrowUp") {
          e.preventDefault();
          setSlashIndex(
            (i) => (i - 1 + filteredItems.length) % filteredItems.length,
          );
          return;
        }
        if (e.key === "Enter" || e.key === "Tab") {
          e.preventDefault();
          applySlashSelection(filteredItems[slashIndex]);
          return;
        }
        if (e.key === "Escape") {
          e.preventDefault();
          closeSlash();
          return;
        }
      }

      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        handleSendClick();
      }
    },
    [
      slashOpen,
      filteredItems,
      slashIndex,
      handleSendClick,
      applySlashSelection,
      closeSlash,
    ],
  );

  const handleInput = useCallback(
    (e: React.FormEvent<HTMLTextAreaElement>) => {
      const target = e.currentTarget;
      updateText(target);
      checkSlash(target.value);
    },
    [checkSlash, updateText],
  );

  const handleStopClick = useCallback(() => {
    onStop();
  }, [onStop]);

  return (
    <div className="px-4 pt-2 shrink-0">
      {images.length > 0 && (
        <div className="mb-2">
          <ChatImages
            novelId={novelId}
            images={images}
            onRemove={(index) =>
              updateImages(images.filter((_, i) => i !== index))
            }
            onVariantChange={(index, variant) =>
              updateImages(
                images.map((image, i) =>
                  i === index ? { ...image, variant } : image,
                ),
              )
            }
          />
          {!supportsVision && (
            <p role="status" className="mt-1 text-xs text-destructive">
              {t("chat.visionRequired")}
            </p>
          )}
        </div>
      )}
      {isImporting && (
        <p role="status" className="mb-1 text-xs text-muted-foreground">
          {t("chat.importingImages")}
        </p>
      )}
      {activeCommand && (
        <div className="flex items-center mb-1 px-1">
          <span
            className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-xs font-medium ${
              activeCommand.type === "manual"
                ? "bg-tag-blue text-tag-blue-foreground"
                : activeCommand.type === "always"
                  ? "bg-tag-green text-tag-green-foreground"
                  : "bg-tag-amber text-tag-amber-foreground"
            }`}
          >
            {activeCommand.type === "manual" ? (
              <Play className="w-3 h-3" />
            ) : activeCommand.type === "always" ? (
              <Star className="w-3 h-3" />
            ) : (
              <Zap className="w-3 h-3" />
            )}
            {activeCommand.name}
          </span>
        </div>
      )}
      <div className="flex items-end gap-2 bg-muted/30 rounded-2xl border px-2 py-2">
        <textarea
          ref={textareaRef}
          placeholder={placeholder}
          disabled={disabled}
          rows={1}
          onKeyDown={handleKeyDown}
          onInput={handleInput}
          onPaste={(e) => {
            const files = Array.from(e.clipboardData.items)
              .filter(
                (item) =>
                  item.kind === "file" && item.type.startsWith("image/"),
              )
              .map((item) => item.getAsFile())
              .filter((file): file is File => file !== null);
            if (files.length) {
              e.preventDefault();
              void importImages(files);
            }
          }}
          className="flex-1 bg-transparent resize-none text-sm leading-relaxed text-foreground outline-none placeholder:text-muted-foreground/50 disabled:text-muted-foreground/40 py-2 px-2 min-h-[28px] max-h-[180px]"
        />
        {isLoading && !hasContent && images.length === 0 ? (
          <button
            aria-label={t("chat.stopGeneration")}
            onClick={handleStopClick}
            disabled={isCancelling}
            className="w-[52px] h-[36px] min-w-[52px] flex items-center justify-center rounded-xl bg-destructive text-destructive-foreground shadow-md transition-all hover:bg-destructive/85 disabled:opacity-70 shrink-0"
          >
            {isCancelling ? (
              <Loader2 className="w-4 h-4 animate-spin" />
            ) : (
              <Square className="w-4 h-4" fill="currentColor" />
            )}
          </button>
        ) : (
          <button
            aria-label={t("chat.sendMessage")}
            disabled={
              disabled ||
              isImporting ||
              (!hasContent && images.length === 0) ||
              (images.length > 0 && !supportsVision)
            }
            onClick={handleSendClick}
            className="w-[52px] h-[36px] min-w-[52px] flex items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-md shadow-primary/20 transition-all hover:-translate-y-px hover:shadow-lg hover:shadow-primary/30 disabled:bg-muted disabled:text-muted-foreground/40 disabled:shadow-none disabled:hover:translate-y-0 shrink-0"
          >
            <ArrowUp className="w-5 h-5" />
          </button>
        )}
      </div>

      {slashOpen && filteredItems.length > 0 && (
        <SlashMenu
          slashItems={filteredItems}
          selectedIndex={slashIndex}
          position={slashPos}
          onSelect={applySlashSelection}
          onHover={setSlashIndex}
        />
      )}
    </div>
  );
}

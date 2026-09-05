import { useState, useCallback, useRef, useEffect, useMemo } from "react";
import { ArrowUp, Square, Zap, Play, Star, Loader2 } from "lucide-react";
import type { app } from "@/lib/wailsjs/go/models";
import SlashMenu from "./SlashMenu";

// 草稿 localStorage key 前缀（per-session 持久化，沿用 goink_ 惯例）
const DRAFT_PREFIX = "goink_chat_draft_";

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
  disabled: boolean;
  isLoading: boolean;
  isCancelling: boolean;
  placeholder: string;
  draftKey: string;
  slashItems: app.SlashCommand[];
  onSend: (message: string) => void;
  onStop: () => void;
  onListSlash: () => void;
}

export default function ChatInput({
  disabled,
  isLoading,
  isCancelling,
  placeholder,
  draftKey,
  slashItems,
  onSend,
  onStop,
  onListSlash,
}: Props) {
  const [hasContent, setHasContent] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const draftTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastDraftKeyRef = useRef(draftKey);

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
        localStorage.setItem(DRAFT_PREFIX + lastDraftKeyRef.current, textarea.value);
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
    [closeSlash],
  );

  const updateSlashPos = useCallback(() => {
    const textarea = textareaRef.current;
    if (!textarea) return;
    const rect = textarea.getBoundingClientRect();
    setSlashPos({ top: rect.top, left: rect.left, width: rect.width });
  }, []);

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
        const textarea = e.currentTarget as HTMLTextAreaElement;
        const value = textarea.value.trim();
        if (value && !disabled) {
          onSend(value);
          textarea.value = "";
          textarea.style.height = "auto";
          setHasContent(false);
          setActiveCommand(null);
          closeSlash();
          // 发送成功：清掉已落盘/待写入的草稿
          if (draftTimerRef.current) {
            clearTimeout(draftTimerRef.current);
            draftTimerRef.current = null;
          }
          localStorage.removeItem(DRAFT_PREFIX + lastDraftKeyRef.current);
        }
      }
    },
    [
      slashOpen,
      filteredItems,
      slashIndex,
      disabled,
      onSend,
      applySlashSelection,
      closeSlash,
    ],
  );

  const handleInput = useCallback(
    (e: React.FormEvent<HTMLTextAreaElement>) => {
      const target = e.currentTarget;
      target.style.height = "auto";
      target.style.height = Math.min(target.scrollHeight, 180) + "px";
      setHasContent(target.value.trim().length > 0);
      checkSlash(target.value);
      // 草稿持久化：防抖 300ms 落盘（与 useLayoutState 同风格）
      if (draftTimerRef.current) clearTimeout(draftTimerRef.current);
      draftTimerRef.current = setTimeout(() => {
        draftTimerRef.current = null;
        localStorage.setItem(
          DRAFT_PREFIX + lastDraftKeyRef.current,
          target.value,
        );
      }, 300);
    },
    [checkSlash],
  );

  const handleSendClick = useCallback(() => {
    const textarea = textareaRef.current;
    if (!textarea) return;
    const value = textarea.value.trim();
    if (value && !disabled) {
      onSend(value);
      textarea.value = "";
      textarea.style.height = "auto";
      setHasContent(false);
      setActiveCommand(null);
      closeSlash();
      // 发送成功：清掉已落盘/待写入的草稿
      if (draftTimerRef.current) {
        clearTimeout(draftTimerRef.current);
        draftTimerRef.current = null;
      }
      localStorage.removeItem(DRAFT_PREFIX + lastDraftKeyRef.current);
    }
  }, [disabled, onSend, closeSlash]);

  const handleStopClick = useCallback(() => {
    onStop();
  }, [onStop]);

  return (
    <div className="px-4 pt-2 shrink-0">
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
          className="flex-1 bg-transparent resize-none text-sm leading-relaxed text-foreground outline-none placeholder:text-muted-foreground/50 disabled:text-muted-foreground/40 py-2 px-2 min-h-[28px] max-h-[180px]"
        />
        {isLoading && !hasContent ? (
          <button
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
            disabled={disabled || !hasContent}
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

import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import Markdown from "@/components/Markdown";
import { useEditorTabsStore } from "./useEditorTabsStore";

interface Props {
  content: string;
  // 位置键 `novelId:path:mode`：按 (tab, mode) 独立保存/恢复滚动位置。不传则不记忆。
  positionKey?: string;
}

export default function OutlineViewer({ content, positionKey }: Props) {
  const { t } = useTranslation();
  const containerRef = useRef<HTMLDivElement>(null);
  const restoredRef = useRef(false);

  // 内容就绪后恢复滚动位置（outlineContent 先空后异步载入，故等首次有内容再恢复）
  useEffect(() => {
    if (!content || !positionKey || restoredRef.current) return;
    restoredRef.current = true;
    const saved =
      useEditorTabsStore.getState().positions[positionKey]?.scrollTop;
    const el = containerRef.current;
    if (el && typeof saved === "number") {
      el.scrollTop = saved;
    }
  }, [content, positionKey]);

  if (!content) {
    return (
      <div className="flex items-center justify-center h-full">
        <p className="text-sm text-muted-foreground">
          {t("content.noOutline")}
        </p>
      </div>
    );
  }

  // 同步写入 store（内存）——localStorage 落盘由 store 全局 subscribe 防抖负责。
  const onScroll = () => {
    if (!positionKey) return;
    const el = containerRef.current;
    if (!el) return;
    useEditorTabsStore.getState().setPosition(positionKey, {
      scrollTop: el.scrollTop,
      updatedAt: Date.now(),
    });
  };

  return (
    <div
      ref={containerRef}
      className="overflow-auto h-full p-6"
      onScroll={onScroll}
    >
      <Markdown content={content} />
    </div>
  );
}

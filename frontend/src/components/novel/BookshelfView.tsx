import { useState, useRef } from "react";
import { useTranslation } from "react-i18next";
import {
  Plus,
  Pencil,
  Trash2,
  BookOpen,
  Camera,
  Download,
  Upload,
} from "lucide-react";
import BookCover from "@/components/sidebar/BookCover";
import { useNovels } from "@/components/novel/useNovels";
import { useNovelStore } from "@/components/novel/useNovelStore";
import type { novel } from "@/lib/wailsjs/go/models";
import { toErrorMessage } from "@/utils/error";

interface Props {
  onSelectNovel: (n: novel.Novel) => void;
  onSaveCover: (novelID: number, file: File) => Promise<void>;
  onImportNovel: () => void;
}

export default function BookshelfView({
  onSelectNovel,
  onSaveCover,
  onImportNovel,
}: Props) {
  const { t } = useTranslation();
  // 3.2: 数据 + UI 状态从 store/query 订阅（原由 WorkspaceView 注入 props）。
  // useNovels 与 WorkspaceView 共享 novelKeys.all 缓存，不重复 fetch。
  const { data: novels = [] } = useNovels();
  const activeNovelId = useNovelStore((s) => s.activeNovelId);
  const setEditingNovel = useNovelStore((s) => s.setEditingNovel);
  const setDeletingNovel = useNovelStore((s) => s.setDeletingNovel);
  const setShowCreateDialog = useNovelStore((s) => s.setShowCreateDialog);
  const setExportNovelId = useNovelStore((s) => s.setExportNovelId);
  const [coverKeys, setCoverKeys] = useState<Record<number, number>>({});
  const [coverError, setCoverError] = useState<Record<number, string>>({});
  const fileInputRef = useRef<HTMLInputElement>(null);
  const uploadingRef = useRef<number | null>(null);

  function triggerCoverUpload(novelID: number) {
    uploadingRef.current = novelID;
    setCoverError((prev) => {
      const next = { ...prev };
      delete next[novelID];
      return next;
    });
    fileInputRef.current?.click();
  }

  function handleCoverClick(novelID: number, e: React.MouseEvent) {
    e.stopPropagation();
    triggerCoverUpload(novelID);
  }

  async function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file || uploadingRef.current == null) return;
    const novelID = uploadingRef.current;
    uploadingRef.current = null;
    // 清空 input 以便重复选同一文件
    e.target.value = "";
    try {
      await onSaveCover(novelID, file);
      setCoverError((prev) => {
        const next = { ...prev };
        delete next[novelID];
        return next;
      });
      setCoverKeys((prev) => ({
        ...prev,
        [novelID]: (prev[novelID] ?? 0) + 1,
      }));
    } catch (err) {
      setCoverError((prev) => ({
        ...prev,
        [novelID]: toErrorMessage(err, t("novel.coverSaveFailed")),
      }));
    }
  }

  return (
    <div className="flex-1 flex flex-col min-h-0 bg-background">
      {/* 隐藏文件选择器 */}
      <input
        ref={fileInputRef}
        type="file"
        accept="image/*"
        className="hidden"
        onChange={handleFileChange}
      />

      {/* 顶部工具栏 */}
      <div className="flex items-center justify-between px-6 py-4 border-b shrink-0">
        <span className="text-sm text-muted-foreground">
          {t("novel.totalWorks", { count: novels.length })}
        </span>
        <div className="flex items-center gap-2">
          <button
            onClick={onImportNovel}
            className="inline-flex items-center gap-1.5 h-8 px-3 rounded-md text-sm border hover:bg-muted transition-colors"
          >
            <Upload className="w-4 h-4" />
            {t("novel.importBook")}
          </button>
          <button
            onClick={() => setShowCreateDialog(true)}
            className="inline-flex items-center gap-1.5 h-8 px-3 rounded-md text-sm bg-primary text-primary-foreground hover:opacity-90 transition-opacity"
          >
            <Plus className="w-4 h-4" />
            {t("novel.newWork")}
          </button>
        </div>
      </div>

      {/* 空状态 */}
      {novels.length === 0 ? (
        <div className="flex-1 flex flex-col items-center justify-center text-muted-foreground gap-3">
          <BookOpen className="w-12 h-12 opacity-30" />
          <p className="text-sm">{t("novel.noWorksYet")}</p>
        </div>
      ) : (
        /* 书架网格 */
        <div className="flex-1 overflow-y-auto overscroll-contain p-6">
          <div className="grid grid-cols-[repeat(auto-fill,minmax(180px,1fr))] gap-5">
            {novels.map((n) => (
              <div
                key={n.id}
                className={`group relative flex flex-col rounded-lg border bg-card hover:shadow-md transition-shadow cursor-pointer select-none
                  ${n.id === activeNovelId ? "ring-2 ring-primary" : ""}`}
              >
                {/* 点击卡片主体切换书 */}
                <div
                  className="flex flex-col flex-1 p-3"
                  onClick={() => onSelectNovel(n)}
                >
                  <div className="w-full aspect-[3/4] mb-3 rounded-sm overflow-hidden relative">
                    <BookCover novelId={n.id} refreshKey={coverKeys[n.id]} />
                    {/* 悬浮封面上传按钮 */}
                    <button
                      onClick={(e) => handleCoverClick(n.id, e)}
                      className="absolute inset-0 flex items-center justify-center bg-black/40 opacity-0 group-hover:opacity-100 transition-opacity"
                      title={t("novel.changeCover")}
                    >
                      <Camera className="w-5 h-5 text-white" />
                    </button>
                  </div>
                  {coverError[n.id] && (
                    <div className="text-xs text-destructive flex items-center gap-2 mb-1">
                      <span className="truncate">{coverError[n.id]}</span>
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          triggerCoverUpload(n.id);
                        }}
                        className="text-primary hover:underline shrink-0"
                      >
                        {t("common.retry")}
                      </button>
                    </div>
                  )}
                  <h3 className="text-sm font-medium truncate mb-1">
                    {n.title}
                  </h3>
                  {n.genre ? (
                    <span className="inline-block self-start text-[11px] px-1.5 py-0.5 rounded bg-primary/10 text-primary mb-1.5">
                      {n.genre}
                    </span>
                  ) : (
                    <span className="inline-block self-start text-[11px] px-1.5 py-0.5 rounded bg-muted text-muted-foreground mb-1.5">
                      {t("novel.uncategorized")}
                    </span>
                  )}
                  {n.description && (
                    <p className="text-xs text-muted-foreground line-clamp-2 leading-relaxed">
                      {n.description}
                    </p>
                  )}
                </div>

                {/* 悬浮操作按钮 */}
                <div className="absolute top-2 right-2 flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      setExportNovelId(n.id);
                    }}
                    className="w-7 h-7 flex items-center justify-center rounded-md bg-background/90 border shadow-sm hover:bg-muted transition-colors"
                    title={t("novel.export")}
                  >
                    <Download className="w-3.5 h-3.5 text-muted-foreground" />
                  </button>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      setEditingNovel(n);
                    }}
                    className="w-7 h-7 flex items-center justify-center rounded-md bg-background/90 border shadow-sm hover:bg-muted transition-colors"
                    title={t("novel.edit")}
                  >
                    <Pencil className="w-3.5 h-3.5 text-muted-foreground" />
                  </button>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      setDeletingNovel(n);
                    }}
                    className="w-7 h-7 flex items-center justify-center rounded-md bg-background/90 border shadow-sm hover:bg-danger-bg hover:border-danger-border transition-colors"
                    title={t("novel.delete")}
                  >
                    <Trash2 className="w-3.5 h-3.5 text-muted-foreground hover:text-destructive" />
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

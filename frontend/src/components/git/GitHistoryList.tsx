import {
  GitGraph,
  Loader2,
  FileText,
  ChevronDown,
  ChevronRight,
  HelpCircle,
} from "lucide-react";
import { useState, useEffect, useRef } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { useInView } from "react-intersection-observer";
import { useTimeAgo } from "@/hooks/useTimeAgo";
import type { git } from "@/lib/wailsjs/go/models";
import GitCommitTooltip from "./GitCommitTooltip";
import { useInfiniteCommitLog } from "./useInfiniteCommitLog";
import { useCommitFiles } from "./useCommitFiles";
import { useFileDiff } from "./useFileDiff";

interface Props {
  novelId: number;
  onSelectFile: (file: git.FileDiff) => void;
}

const PAGE_SIZE = 50;

export default function GitHistoryList({ novelId, onSelectFile }: Props) {
  const { t, i18n } = useTranslation();
  // 相对时间：每分钟自动刷新
  const timeAgo = useTimeAgo();
  const [hoveredHash, setHoveredHash] = useState<string | null>(null);
  const [tooltipRect, setTooltipRect] = useState<{
    top: number;
    height: number;
  } | null>(null);
  const [expandedHash, setExpandedHash] = useState<string | null>(null);
  const [selectedFilePath, setSelectedFilePath] = useState<string | null>(null);
  const [showHelp, setShowHelp] = useState(false);
  const [helpPos, setHelpPos] = useState({ top: 0, left: 0 });
  const helpIconRef = useRef<HTMLButtonElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const hideTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // 无限滚动：sentinel 进入视口时拉下一页（react-intersection-observer useInView）
  const { ref: sentinelRef, inView } = useInView({ rootMargin: "100px" });

  // 父组件 handleSelectGitFile 未 useCallback 包装，引用每次 render 都变，
  // 用 ref 包避免 useEffect 依赖 onSelectFile 重复触发（Dan Abramov 推荐模式）。
  const onSelectFileRef = useRef(onSelectFile);
  useEffect(() => {
    onSelectFileRef.current = onSelectFile;
  });

  // 提交历史无限滚动 query（游标分页：afterHash）
  const commitsQuery = useInfiniteCommitLog({
    novelId,
    size: PAGE_SIZE,
    enabled: true,
  });
  const commits = commitsQuery.data?.pages.flatMap((p) => p) ?? [];

  // 展开 commit 的文件列表（按需，enabled 守卫：!!expandedHash）
  const commitFilesQuery = useCommitFiles(novelId, expandedHash);
  const expandedFiles = commitFilesQuery.data ?? [];

  // 选中文件的 diff（按需，enabled 守卫：!!expandedHash && !!selectedFilePath）
  const fileDiffQuery = useFileDiff(novelId, expandedHash, selectedFilePath);

  // 文件列表返回后自动选中第一个文件（触发 useFileDiff refetch）
  useEffect(() => {
    if (!expandedHash) return;
    if (commitFilesQuery.data && commitFilesQuery.data.length > 0) {
      const first = commitFilesQuery.data[0].path;
      setSelectedFilePath((prev) => (prev === first ? prev : first));
    } else if (commitFilesQuery.data && commitFilesQuery.data.length === 0) {
      setSelectedFilePath((prev) => (prev === null ? prev : null));
    }
  }, [expandedHash, commitFilesQuery.data]);

  // diff 数据返回后上传父组件 GitCommitView（用 ref 包避免 onSelectFile 引用变化重复触发）
  useEffect(() => {
    if (fileDiffQuery.data && expandedHash && selectedFilePath) {
      onSelectFileRef.current(fileDiffQuery.data);
    }
  }, [fileDiffQuery.data, expandedHash, selectedFilePath]);

  // 切 novelId 时收起展开态（query 自动失效，但 UI 态需手动清）
  useEffect(() => {
    setExpandedHash(null);
    setSelectedFilePath(null);
    setHoveredHash(null);
    setTooltipRect(null);
  }, [novelId]);

  useEffect(() => {
    return () => {
      if (hideTimerRef.current) clearTimeout(hideTimerRef.current);
    };
  }, []);

  // 滚动时隐藏 tooltip，防止位置偏移
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const onScroll = () => {
      setHoveredHash(null);
      setTooltipRect(null);
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => el.removeEventListener("scroll", onScroll);
  }, []);

  // 无限滚动：sentinel 进入视口时拉下一页（react-intersection-observer useInView）
  useEffect(() => {
    if (inView && commitsQuery.hasNextPage && !commitsQuery.isFetchingNextPage) {
      commitsQuery.fetchNextPage();
    }
  }, [
    inView,
    commitsQuery.hasNextPage,
    commitsQuery.isFetchingNextPage,
    commitsQuery.fetchNextPage,
  ]);

  // 展开/收起 commit
  function toggleExpand(hash: string) {
    if (expandedHash === hash) {
      setExpandedHash(null);
      setSelectedFilePath(null);
      setHoveredHash(null);
      setTooltipRect(null);
      return;
    }
    setExpandedHash(hash);
    setSelectedFilePath(null);
    setHoveredHash(null);
    setTooltipRect(null);
  }

  function handleSelectFile(entry: git.FileEntry) {
    setSelectedFilePath(entry.path);
    setHoveredHash(null);
    setTooltipRect(null);
  }

  function renderTime(commit: git.CommitInfo) {
    const relative = timeAgo(commit.time);
    const d = new Date(commit.time);
    const dateStr = new Intl.DateTimeFormat(i18n.language, {
      year: "numeric",
      month: "long",
      day: "numeric",
    }).format(d);
    const timeStr = `${d.getHours().toString().padStart(2, "0")}:${d.getMinutes().toString().padStart(2, "0")}`;
    const full = `${dateStr} ${timeStr}`;
    return { relative, full };
  }

  // 通过 portal 渲染 tooltip，定位不依赖滚动容器
  function renderTooltip() {
    if (!hoveredHash || !tooltipRect) return null;
    const commit = commits.find((c) => c.hash === hoveredHash);
    if (!commit) return null;

    // ActivityBar w-12 (3rem) + SidePanel w-56 (14rem) = 17rem
    const sidebarRightEdgeRem = 17;
    return createPortal(
      <div
        className="fixed z-50"
        style={{
          left: `${sidebarRightEdgeRem + 0.5}rem`,
          top: tooltipRect.top + tooltipRect.height / 2,
          transform: "translateY(-50%)",
        }}
        onMouseEnter={() => {
          if (hideTimerRef.current) clearTimeout(hideTimerRef.current);
        }}
        onMouseLeave={() => {
          setHoveredHash(null);
          setTooltipRect(null);
        }}
      >
        <div className="relative">
          <div className="absolute -left-1.5 top-1/2 -translate-y-1/2 w-3 h-3 bg-card border-l border-t border-border rotate-45" />
          <GitCommitTooltip commit={commit} />
        </div>
      </div>,
      document.body,
    );
  }

  const isLoading = commitsQuery.isPending;
  const isError = commitsQuery.isError;
  const loadingMore = commitsQuery.isFetchingNextPage;
  const hasMore = commitsQuery.hasNextPage;
  const loadingFiles = expandedHash !== null && commitFilesQuery.isPending;
  const loadingDiff = fileDiffQuery.isPending;

  return (
    <>
      <div className="flex items-center gap-2 px-3 py-2.5 border-b">
        <GitGraph className="w-4 h-4 text-muted-foreground" />
        <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          {t("git.creationHistory")}
        </span>
        <button
          ref={helpIconRef}
          onClick={() => {
            if (!showHelp && helpIconRef.current) {
              const r = helpIconRef.current.getBoundingClientRect();
              setHelpPos({ top: r.top + r.height / 2, left: r.right });
            }
            setShowHelp((v) => !v);
          }}
          className="ml-0.5 w-4 h-4 flex items-center justify-center rounded text-muted-foreground/40 hover:text-muted-foreground hover:bg-muted transition-colors cursor-help"
        >
          <HelpCircle className="w-3.5 h-3.5" />
        </button>
        {!isLoading && (
          <span className="text-[10px] text-muted-foreground/50 ml-auto">
            {commits.length}
          </span>
        )}
      </div>

      {showHelp &&
        createPortal(
          <div
            className="fixed z-[100] w-64 text-xs leading-relaxed bg-popover text-popover-foreground border rounded-md p-3 shadow-md"
            style={{
              top: helpPos.top,
              left: helpPos.left + 8,
              transform: "translateY(-50%)",
            }}
            onMouseLeave={() => setShowHelp(false)}
          >
            <p className="mb-2">{t("git.helpText1")}</p>
            <p className="mt-2">{t("git.helpText2")}</p>
          </div>,
          document.body,
        )}

      <div
        ref={scrollRef}
        className="flex-1 overflow-y-auto overscroll-contain"
      >
        {isLoading ? (
          <div className="flex items-center justify-center h-full">
            <Loader2 className="w-5 h-5 text-muted-foreground animate-spin" />
          </div>
        ) : isError ? (
          <div className="flex flex-col items-center justify-center h-full px-4 text-center">
            <p className="text-xs text-destructive mb-2">
              {t("git.loadFailed")}
            </p>
            <button
              onClick={() => commitsQuery.refetch()}
              className="text-xs text-primary hover:underline"
            >
              {t("git.retry")}
            </button>
          </div>
        ) : commits.length === 0 ? (
          <div className="flex items-center justify-center h-full">
            <div className="text-center">
              <GitGraph className="w-8 h-8 text-muted-foreground/30 mx-auto mb-2" />
              <p className="text-xs text-muted-foreground">
                {t("git.noCommits")}
              </p>
            </div>
          </div>
        ) : (
          <div>
            {commits.map((commit) => {
              const isExpanded = expandedHash === commit.hash;
              const { relative, full } = renderTime(commit);

              return (
                <div key={commit.hash} className="relative">
                  <button
                    onClick={() => toggleExpand(commit.hash)}
                    onMouseEnter={(e) => {
                      if (hideTimerRef.current)
                        clearTimeout(hideTimerRef.current);
                      setHoveredHash(commit.hash);
                      const r = e.currentTarget.getBoundingClientRect();
                      setTooltipRect({ top: r.top, height: r.height });
                    }}
                    onMouseLeave={() => {
                      hideTimerRef.current = setTimeout(() => {
                        setHoveredHash(null);
                        setTooltipRect(null);
                      }, 300);
                    }}
                    className={`w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-muted/50 transition-colors relative
                      ${isExpanded ? "bg-primary/10" : ""}`}
                  >
                    {isExpanded && (
                      <span className="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-6 bg-primary rounded-r-full" />
                    )}
                    <div className="shrink-0 w-4 flex items-center justify-center">
                      {loadingFiles && isExpanded ? (
                        <Loader2 className="w-3 h-3 text-muted-foreground animate-spin" />
                      ) : isExpanded ? (
                        <ChevronDown className="w-3.5 h-3.5 text-muted-foreground/60" />
                      ) : (
                        <ChevronRight className="w-3.5 h-3.5 text-muted-foreground/60" />
                      )}
                    </div>
                    <FileText className="w-3.5 h-3.5 text-muted-foreground shrink-0 mt-0.5 self-start" />
                    <div className="flex-1 min-w-0">
                      <p className="text-sm truncate">
                        {commit.message || "(no message)"}
                      </p>
                      <p className="text-[11px] text-muted-foreground mt-0.5">
                        <span className="font-medium">{commit.authorName}</span>
                        <span className="mx-1 text-muted-foreground/30">·</span>
                        <span title={full}>{relative}</span>
                      </p>
                    </div>
                    <code className="text-[10px] font-mono text-muted-foreground/50 shrink-0">
                      {commit.shortHash}
                    </code>
                  </button>

                  {/* 展开错误占位 */}
                  {isExpanded && commitFilesQuery.isError && (
                    <div className="border-l-2 border-primary/20 ml-5 mb-1">
                      <div className="text-xs text-destructive flex items-center gap-2 px-7 py-1.5">
                        <span>{t("git.expandCommitFailed")}</span>
                        <button
                          onClick={() => commitFilesQuery.refetch()}
                          className="text-primary hover:underline"
                        >
                          {t("common.retry")}
                        </button>
                      </div>
                    </div>
                  )}
                  {/* 展开的文件列表 */}
                  {isExpanded &&
                    !commitFilesQuery.isError &&
                    expandedFiles.length > 0 && (
                      <div className="border-l-2 border-primary/20 ml-5 mb-1">
                        {expandedFiles.map((file) => {
                          const isActive = selectedFilePath === file.path;
                          const tagColor =
                            file.changeType === "added"
                              ? "bg-tag-green text-tag-green-foreground"
                              : file.changeType === "deleted"
                                ? "bg-tag-rose text-tag-rose-foreground"
                                : file.changeType === "renamed"
                                  ? "bg-tag-blue text-tag-blue-foreground"
                                  : "bg-tag-amber text-tag-amber-foreground";
                          const label =
                            file.changeType === "added"
                              ? "A"
                              : file.changeType === "deleted"
                                ? "D"
                                : file.changeType === "renamed"
                                  ? "R"
                                  : "M";

                          return (
                            <button
                              key={file.path}
                              onClick={() => handleSelectFile(file)}
                              className={`w-full flex items-center gap-2 pl-7 pr-3 py-1.5 text-left hover:bg-muted/40 transition-colors relative
                              ${isActive ? "bg-primary/5 text-foreground font-medium" : ""}`}
                            >
                              {isActive && (
                                <span className="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-4 bg-primary rounded-r-full" />
                              )}
                              <span
                                className={`text-[10px] font-mono font-medium px-1 py-0.5 rounded shrink-0 ${tagColor}`}
                              >
                                {label}
                              </span>
                              <span className="text-xs truncate flex-1 min-w-0">
                                {file.changeType === "renamed" && file.oldPath
                                  ? `${file.oldPath} → ${file.path}`
                                  : file.path}
                              </span>
                              {isActive && loadingDiff && (
                                <Loader2 className="w-3 h-3 text-muted-foreground animate-spin shrink-0" />
                              )}
                            </button>
                          );
                        })}
                      </div>
                    )}
                </div>
              );
            })}

            <div
              ref={sentinelRef}
              className="h-4 flex items-center justify-center"
            >
              {loadingMore && (
                <Loader2 className="w-4 h-4 text-muted-foreground animate-spin" />
              )}
              {!hasMore && commits.length > PAGE_SIZE && (
                <p className="text-[10px] text-muted-foreground/50">
                  {t("git.allCommitsShown")}
                </p>
              )}
            </div>
          </div>
        )}
      </div>

      {renderTooltip()}
    </>
  );
}

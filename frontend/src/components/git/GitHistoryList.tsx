import {
  GitGraph,
  Loader2,
  FileText,
  ChevronDown,
  ChevronRight,
  HelpCircle,
} from "lucide-react";
import { useState, useEffect, useCallback, useRef } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { useTimeAgo } from "@/hooks/useTimeAgo";
import {
  GetCommitLog,
  GetCommitFileList,
  GetFileDiff,
} from "@/lib/wailsjs/go/app/App";
import type { git } from "@/lib/wailsjs/go/models";
import GitCommitTooltip from "./GitCommitTooltip";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";

interface Props {
  novelId: number;
  onSelectFile: (file: git.FileDiff) => void;
}

const PAGE_SIZE = 50;

export default function GitHistoryList({ novelId, onSelectFile }: Props) {
  const { t, i18n } = useTranslation();
  // 相对时间：每分钟自动刷新
  const timeAgo = useTimeAgo();
  const [commits, setCommits] = useState<git.CommitInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [hasMore, setHasMore] = useState(true);
  const [loadFailed, setLoadFailed] = useState(false);
  const [hoveredHash, setHoveredHash] = useState<string | null>(null);
  const [tooltipRect, setTooltipRect] = useState<{
    top: number;
    height: number;
  } | null>(null);
  const [expandedHash, setExpandedHash] = useState<string | null>(null);
  const expandedHashRef = useRef<string | null>(null);
  const [expandedFiles, setExpandedFiles] = useState<git.FileEntry[]>([]);
  const [loadingFiles, setLoadingFiles] = useState(false);
  const [selectedFilePath, setSelectedFilePath] = useState<string | null>(null);
  const selectedFilePathRef = useRef<string | null>(null);
  const [loadingDiff, setLoadingDiff] = useState(false);
  const [expandedError, setExpandedError] = useState<string | null>(null);
  const [showHelp, setShowHelp] = useState(false);
  const [helpPos, setHelpPos] = useState({ top: 0, left: 0 });
  const helpIconRef = useRef<HTMLButtonElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const hideTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const commitsRef = useRef(commits);
  const loadingMoreRef = useRef(loadingMore);
  const hasMoreRef = useRef(hasMore);

  useEffect(() => {
    commitsRef.current = commits;
  }, [commits]);
  useEffect(() => {
    loadingMoreRef.current = loadingMore;
  }, [loadingMore]);
  useEffect(() => {
    hasMoreRef.current = hasMore;
  }, [hasMore]);

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

  const load = useCallback(
    async (n: number, afterHash?: string) => {
      const list = await GetCommitLog(novelId, n, afterHash ?? "");
      return list ?? [];
    },
    [novelId],
  );

  // 初始加载
  useEffect(() => {
    if (!novelId) {
      setCommits([]);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setLoadFailed(false);
    setHasMore(true);
    setExpandedHash(null);
    expandedHashRef.current = null;
    setExpandedFiles([]);
    setSelectedFilePath(null);
    selectedFilePathRef.current = null;

    load(PAGE_SIZE)
      .then((list) => {
        if (cancelled) return;
        setCommits(list);
        setHasMore(list.length >= PAGE_SIZE);
      })
      .catch((err: Error) => {
        if (cancelled) return;
        setLoadFailed(true);
        toastError(
          t("git.loadFailed") +
            ": " +
            (err instanceof Error ? err.message : String(err)),
        );
        console.error(err);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [novelId, load, t]);

  // IntersectionObserver 加载更多（游标翻页）
  useEffect(() => {
    if (!sentinelRef.current || !hasMore || loading) return;

    const observer = new IntersectionObserver(
      async (entries) => {
        const entry = entries[0];
        if (
          !entry.isIntersecting ||
          loadingMoreRef.current ||
          !hasMoreRef.current
        )
          return;

        setLoadingMore(true);
        try {
          const last = commitsRef.current[commitsRef.current.length - 1];
          const cursor = last?.hash;
          const list = await load(PAGE_SIZE, cursor);
          if (list.length > 0) {
            setCommits((prev) => [...prev, ...list]);
            setHasMore(list.length >= PAGE_SIZE);
          } else {
            setHasMore(false);
          }
        } catch {
          /* ignore */
        }
        setLoadingMore(false);
      },
      { rootMargin: "100px" },
    );

    observer.observe(sentinelRef.current);
    return () => observer.disconnect();
  }, [hasMore, load, loading]);

  // 展开/收起 commit
  async function toggleExpand(hash: string) {
    if (expandedHash === hash) {
      setExpandedHash(null);
      expandedHashRef.current = null;
      setExpandedFiles([]);
      setSelectedFilePath(null);
      selectedFilePathRef.current = null;
      setExpandedError(null);
      return;
    }
    setExpandedHash(hash);
    expandedHashRef.current = hash;
    setLoadingFiles(true);
    setExpandedFiles([]);
    setSelectedFilePath(null);
    selectedFilePathRef.current = null;
    setExpandedError(null);
    setHoveredHash(null);
    try {
      const result = await GetCommitFileList(novelId, hash);
      // 响应返回时确认用户仍在查看同一个 commit（用 ref 避免闭包旧值问题）
      if (expandedHashRef.current !== hash) return;
      const files = result?.files ?? [];
      setExpandedFiles(files);
      // 自动选中第一个文件并懒加载其 diff
      if (files.length > 0) {
        setSelectedFilePath(files[0].path);
        selectedFilePathRef.current = files[0].path;
        loadFileDiff(hash, files[0].path);
      }
    } catch (err) {
      setExpandedError(toErrorMessage(err, t("git.expandCommitFailed")));
    }
    if (expandedHashRef.current === hash) setLoadingFiles(false);
  }

  // 懒加载单个文件的 diff 内容
  async function loadFileDiff(hash: string, filePath: string) {
    setLoadingDiff(true);
    try {
      const diff = await GetFileDiff(novelId, hash, filePath);
      if (
        diff &&
        selectedFilePathRef.current === filePath &&
        expandedHashRef.current === hash
      ) {
        onSelectFile(diff);
      }
    } catch (err) {
      toastError(toErrorMessage(err, t("git.loadDiffFailed")));
    }
    if (expandedHashRef.current === hash) setLoadingDiff(false);
  }

  function handleSelectFile(entry: git.FileEntry) {
    setSelectedFilePath(entry.path);
    selectedFilePathRef.current = entry.path;
    setHoveredHash(null);
    setTooltipRect(null);
    if (expandedHash) {
      loadFileDiff(expandedHash, entry.path);
    }
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
        {!loading && (
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
        {loading ? (
          <div className="flex items-center justify-center h-full">
            <Loader2 className="w-5 h-5 text-muted-foreground animate-spin" />
          </div>
        ) : loadFailed ? (
          <div className="flex flex-col items-center justify-center h-full px-4 text-center">
            <p className="text-xs text-destructive mb-2">
              {t("git.loadFailed")}
            </p>
            <button
              onClick={() => {
                setLoadFailed(false);
                setLoading(true);
                load(PAGE_SIZE)
                  .then((list) => {
                    setCommits(list);
                    setHasMore(list.length >= PAGE_SIZE);
                  })
                  .catch((err: Error) => {
                    setLoadFailed(true);
                    toastError(
                      t("git.loadFailed") +
                        ": " +
                        (err instanceof Error ? err.message : String(err)),
                    );
                    console.error(err);
                  })
                  .finally(() => setLoading(false));
              }}
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
                  {isExpanded && expandedError && (
                    <div className="border-l-2 border-primary/20 ml-5 mb-1">
                      <div className="text-xs text-destructive flex items-center gap-2 px-7 py-1.5">
                        <span>{expandedError}</span>
                        <button
                          onClick={() => toggleExpand(commit.hash)}
                          className="text-primary hover:underline"
                        >
                          {t("common.retry")}
                        </button>
                      </div>
                    </div>
                  )}
                  {/* 展开的文件列表 */}
                  {isExpanded && !expandedError && expandedFiles.length > 0 && (
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

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  Loader2,
  RefreshCw,
  Store,
  X,
  AlertTriangle,
  ExternalLink,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import type { remote } from "@/lib/wailsjs/go/models";
import Markdown from "@/components/Markdown";
import { splitFrontmatter } from "@/components/content/types";
import { BrowserOpenURL } from "@/lib/wailsjs/runtime/runtime";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";
import { skillKeys } from "@/lib/queryKeys";
import { AppErr } from "@/utils/wailsResult";
import { useSkills } from "./useSkills";
import { useRemoteSkills } from "./useRemoteSkills";
import {
  useRemoteSkillContent,
  fetchRemoteSkillContent,
} from "./useRemoteSkillContent";
import { useInstallRemoteSkill } from "./useInstallRemoteSkill";
import { useFileContent } from "@/components/content/useFileContent";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  novelId: number;
  onInstalled?: () => void;
}

type Phase = "browse" | "detail" | "confirm_overwrite";
type InstallTarget = "user" | "novel";

const REPO_URL = "https://github.com/sigpanic/goink-skills";
const PAGE_SIZE_OPTIONS = [10, 20, 50];
const DEFAULT_PAGE_SIZE = 20;
const DEBOUNCE_MS = 300;

function pathForSource(target: InstallTarget, name: string): string {
  return target === "user" ? `~/.goink/skills/${name}.md` : `skills/${name}.md`;
}

function modeClass(mode: string): string {
  if (mode === "manual") return "bg-tag-blue text-tag-blue-foreground";
  if (mode === "always") return "bg-tag-green text-tag-green-foreground";
  return "bg-tag-amber text-tag-amber-foreground";
}

function modeLabel(t: TFunction, mode: string): string {
  if (mode === "manual") return t("skill.marketplace.modeCommand");
  if (mode === "always") return t("skill.marketplace.modePermanent");
  return t("skill.marketplace.modeSmart");
}

function classifyError(
  code: string,
  msg: string,
  t: TFunction,
): { code: string; message: string } {
  // 后端 err_code 是带模块前缀的全码（如 "githubapi.rate_limited"、"llm.not_found"），
  // 用 endsWith 匹配短码，保持 canRetry 等下游逻辑用短码判断。
  if (code.endsWith("network"))
    return { code: "network", message: t("skill.marketplace.errorNetwork") };
  if (code.endsWith("rate_limited"))
    return {
      code: "rate_limited",
      message: t("skill.marketplace.errorRateLimited"),
    };
  if (code.endsWith("not_found"))
    return { code: "not_found", message: t("skill.marketplace.errorNotFound") };
  if (code.endsWith("forbidden"))
    return {
      code: "forbidden",
      message: t("skill.marketplace.errorForbidden"),
    };
  return {
    code: "other",
    message: t("skill.marketplace.errorOther", { message: msg || code }),
  };
}

// 从 query.error 提取 classifyError 结果（apperr 新 API 的 AppErr 带 errCode）。
// query.error 是 unknown，用 instanceof AppErr 守卫提取 errCode；非 AppErr 时 code="" 走 other 分支。
function classifyQueryError(
  err: unknown,
  t: TFunction,
): { code: string; message: string } | null {
  if (!err) return null;
  const code = err instanceof AppErr ? err.errCode : "";
  return classifyError(code, toErrorMessage(err), t);
}

export default function SkillMarketplace({
  open,
  onOpenChange,
  novelId,
  onInstalled,
}: Props) {
  const qc = useQueryClient();
  const { t } = useTranslation();
  const { fetchContent } = useFileContent();

  const [phase, setPhase] = useState<Phase>("browse");
  const [query, setQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);

  const [selectedSkill, setSelectedSkill] =
    useState<remote.RemoteSkillMeta | null>(null);
  // localContent / remoteContentForConfirm / contentError 是 confirm_overwrite phase 用的 local state：
  // detail phase 的 remote content 走 useRemoteSkillContent query，confirm_overwrite 时拷贝过来。
  const [localContent, setLocalContent] = useState("");
  const [remoteContentForConfirm, setRemoteContentForConfirm] = useState("");
  const [contentError, setContentError] = useState("");

  const [installTarget, setInstallTarget] = useState<InstallTarget>("user");
  // 5.4 commit 4: installing 由 mutation.isPending 推导（删 useState），mutation onSuccess 失效 skills + remote-skills。
  // installTarget 仍需 useState：在 doInstall 之前 setInstallTarget 标记哪个按钮 loading，mutation 不持此信息。
  const installMutation = useInstallRemoteSkill(novelId);
  const installing = installMutation.isPending;

  // 5.4 commit 3: skills（已安装索引）走 useSkills query（commit 1 建），与 SkillList 共享缓存。
  // installedNames/installedVersions 由 query data 推导，删 loadInstalledIndex + useState。
  const { data: installedSkills = [] } = useSkills(novelId);
  const { installedNames, installedVersions } = useMemo(() => {
    const nameSet = new Set<string>();
    const versionMap = new Map<string, number>();
    for (const s of installedSkills) {
      nameSet.add(s.name);
      const prev = versionMap.get(s.name) ?? 0;
      if (s.version > prev) versionMap.set(s.name, s.version);
    }
    return { installedNames: nameSet, installedVersions: versionMap };
  }, [installedSkills]);

  // 5.4 commit 3: 远程技能列表走 useRemoteSkills query（apperr 新 API，unwrapResult throw AppErr）。
  // queryKey 含 page/size/query，debounce 由 debouncedQuery 进 key 实现（queryKey 变化自动 refetch）。
  // enabled: open（modal 关闭时不 fetch，缓存保留 gcTime 供下次 open 快速显示）。
  // error 由 query.error 经 classifyQueryError 映射 inline error bar（保留短码文案），中间件同时弹兜底 toast。
  const remoteListQuery = useRemoteSkills(
    { page, size: pageSize, query: debouncedQuery },
    open,
  );
  const items = remoteListQuery.data?.items ?? [];
  const total = remoteListQuery.data?.total ?? 0;
  const totalPages = remoteListQuery.data?.total_pages ?? 0;
  const loading = remoteListQuery.isLoading;
  const error = useMemo(
    () => classifyQueryError(remoteListQuery.error, t),
    [remoteListQuery.error, t],
  );

  // 5.4 commit 3: 远程技能内容走 useRemoteSkillContent query（apperr 新 API）。
  // enabled: !!selectedSkill && phase === "detail"（confirm_overwrite 不重新 fetch，用 remoteContentForConfirm）。
  const remoteContentQuery = useRemoteSkillContent(
    selectedSkill?.name ?? "",
    !!selectedSkill && phase === "detail",
  );
  const remoteContent = remoteContentQuery.data ?? "";
  const contentLoading = remoteContentQuery.isLoading;
  const detailContentError = useMemo(
    () => classifyQueryError(remoteContentQuery.error, t)?.message ?? "",
    [remoteContentQuery.error, t],
  );

  const canUpdateDetail = useMemo(() => {
    if (!selectedSkill) return false;
    const v = installedVersions.get(selectedSkill.name);
    return v !== undefined && selectedSkill.version > v;
  }, [selectedSkill, installedVersions]);

  const debounceTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Reset local state when modal closes（query 缓存保留，下次 open 快速显示后 refetch）
  useEffect(() => {
    if (!open) {
      setPhase("browse");
      setSelectedSkill(null);
      setLocalContent("");
      setRemoteContentForConfirm("");
      setInstallTarget("user");
      setQuery("");
      setDebouncedQuery("");
      setPage(1);
      setPageSize(DEFAULT_PAGE_SIZE);
      setContentError("");
    }
  }, [open]);

  // Debounce search query
  useEffect(() => {
    if (debounceTimerRef.current) clearTimeout(debounceTimerRef.current);
    debounceTimerRef.current = setTimeout(() => {
      setDebouncedQuery(query);
      setPage(1);
    }, DEBOUNCE_MS);
    return () => {
      if (debounceTimerRef.current) clearTimeout(debounceTimerRef.current);
    };
  }, [query]);

  // Click card → enter detail phase
  const handleCardClick = useCallback((sk: remote.RemoteSkillMeta) => {
    setSelectedSkill(sk);
    setPhase("detail");
    setLocalContent("");
    setRemoteContentForConfirm("");
    setContentError("");
  }, []);

  // GetContent probe for install target（复用 content 领域 useFileContent，共享缓存）
  const probeLocal = useCallback(
    async (target: InstallTarget, name: string): Promise<string> => {
      if (target === "novel" && !novelId) return "";
      const path = pathForSource(target, name);
      try {
        const content = await fetchContent(novelId, path);
        return content || "";
      } catch {
        return "";
      }
    },
    [fetchContent, novelId],
  );

  // Install skill（commit 4: 走 useInstallRemoteSkill mutation + unwrapResult，删 InstallRemoteSkill 直接 import）。
  // mutation onSuccess 已失效 skillKeys.list(novelId) + ["remote-skills"]，doInstall 内不再手动 invalidate。
  // mutateAsync 稳定引用（TanStack Query 保证），进 deps 不会触发 doInstall 重建。
  const installMutateAsync = installMutation.mutateAsync;
  const doInstall = useCallback(
    async (target: InstallTarget, name: string) => {
      try {
        await installMutateAsync({
          name,
          target,
          novel_id: novelId,
        });
        // success — 触发回调，回 browse（query 失效由 mutation onSuccess 处理）
        onInstalled?.();
        setPhase("browse");
        setSelectedSkill(null);
        setLocalContent("");
        setRemoteContentForConfirm("");
      } catch (e: unknown) {
        // 统一到 catch toast（删 err_code 分支）：AppErr 用 classifyError 映射短码文案，
        // 非 AppErr（如网络层错误）走 installFailed + msg 兜底。
        if (e instanceof AppErr) {
          const cls = classifyError(e.errCode, e.message, t);
          toastError(cls.message);
        } else {
          toastError(
            t("skill.marketplace.installFailed") + ": " + toErrorMessage(e),
          );
        }
      }
    },
    [installMutateAsync, novelId, t, onInstalled],
  );

  // Click install button — probe then install or enter confirm_overwrite
  const handleInstall = useCallback(
    async (target: InstallTarget) => {
      if (!selectedSkill) return;
      if (target === "novel" && !novelId) {
        toastError(t("skill.marketplace.novelRequired"));
        return;
      }
      setInstallTarget(target);
      const local = await probeLocal(target, selectedSkill.name);
      if (local) {
        // has same-name local skill → enter confirm_overwrite phase
        setLocalContent(local);
        // 从 useRemoteSkillContent query 缓存取 content，或 fetchQuery 拉取（走同一 queryKey 复用缓存）
        let remote = remoteContent;
        if (!remote) {
          try {
            remote = await qc.fetchQuery({
              queryKey: skillKeys.remoteContent(selectedSkill.name),
              queryFn: () => fetchRemoteSkillContent(selectedSkill.name),
            });
          } catch (e: unknown) {
            const cls = classifyQueryError(e, t);
            setContentError(cls?.message ?? "");
            remote = "";
          }
        }
        setRemoteContentForConfirm(remote);
        setPhase("confirm_overwrite");
      } else {
        await doInstall(target, selectedSkill.name);
      }
    },
    [
      selectedSkill,
      novelId,
      t,
      probeLocal,
      qc,
      doInstall,
      remoteContent,
    ],
  );

  // Confirm overwrite
  const handleConfirmOverwrite = useCallback(async () => {
    if (!selectedSkill) return;
    await doInstall(installTarget, selectedSkill.name);
  }, [selectedSkill, installTarget, doInstall]);

  // Refresh button — invalidate queries（query 自动 refetch）
  const handleRefresh = useCallback(() => {
    qc.invalidateQueries({ queryKey: ["remote-skills"] });
    qc.invalidateQueries({ queryKey: skillKeys.list(novelId) });
  }, [qc, novelId]);

  // Overlay click — only close in browse phase
  const handleOverlayClick = useCallback(() => {
    if (phase === "browse") {
      onOpenChange(false);
    }
  }, [phase, onOpenChange]);

  // Pagination helpers
  const canPrev = page > 1;
  const canNext = page < totalPages;

  const remoteSplit = useMemo(
    () => splitFrontmatter(remoteContent),
    [remoteContent],
  );
  const remoteForConfirmSplit = useMemo(
    () => splitFrontmatter(remoteContentForConfirm),
    [remoteContentForConfirm],
  );
  const localSplit = useMemo(
    () => splitFrontmatter(localContent),
    [localContent],
  );

  if (!open) return null;

  const canRetry =
    error &&
    (error.code === "network" ||
      error.code === "rate_limited" ||
      error.code === "other");

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div
        className="absolute inset-0 bg-black/40"
        onClick={handleOverlayClick}
      />
      <div className="relative bg-background rounded-xl shadow-2xl border w-[min(1600px,96vw)] h-[92vh] max-h-[94vh] flex flex-col">
        {/* Top toolbar — switches by phase */}
        {phase === "browse" && (
          <div className="flex items-center justify-between px-6 py-4 border-b shrink-0 gap-3">
            <div className="flex items-center gap-2">
              <Store className="w-5 h-5 text-primary" />
              <h2 className="text-base font-semibold">
                {t("skill.marketplace.title")}
              </h2>
              <button
                onClick={() => BrowserOpenURL(REPO_URL)}
                className="inline-flex items-center justify-center w-7 h-7 rounded-lg hover:bg-muted transition-colors text-muted-foreground hover:text-foreground"
                title={t("skill.marketplace.repoLink")}
              >
                <ExternalLink className="w-3.5 h-3.5" />
              </button>
            </div>
            <div className="flex items-center gap-2">
              <div className="relative">
                <input
                  type="text"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  placeholder={t("skill.marketplace.search")}
                  className="w-64 h-8 pl-3 pr-3 text-sm bg-muted/40 rounded-lg border-0 outline-none focus:ring-1 focus:ring-ring"
                />
              </div>
              <button
                onClick={handleRefresh}
                disabled={loading}
                className="inline-flex items-center justify-center w-8 h-8 rounded-lg border border-border hover:bg-muted transition-colors disabled:opacity-50"
                title={t("skill.marketplace.refresh")}
              >
                <RefreshCw
                  className={`w-3.5 h-3.5 ${loading ? "animate-spin" : ""}`}
                />
              </button>
              <button
                onClick={() => onOpenChange(false)}
                className="inline-flex items-center justify-center w-8 h-8 rounded-lg border border-border hover:bg-muted transition-colors text-muted-foreground hover:text-foreground"
                title={t("skill.marketplace.close")}
              >
                <X className="w-4 h-4" />
              </button>
            </div>
          </div>
        )}

        {phase === "detail" && selectedSkill && (
          <div className="flex items-center justify-between px-6 py-4 border-b shrink-0 gap-3">
            <div className="flex items-center gap-2 min-w-0">
              <button
                onClick={() => {
                  setPhase("browse");
                  setSelectedSkill(null);
                  setLocalContent("");
                  setContentError("");
                }}
                className="inline-flex items-center justify-center w-8 h-8 rounded-lg border border-border hover:bg-muted transition-colors text-muted-foreground hover:text-foreground shrink-0"
                title={t("skill.marketplace.back")}
              >
                <ArrowLeft className="w-4 h-4" />
              </button>
              <h2 className="text-base font-semibold truncate">
                {selectedSkill.name}
                <span className="ml-2 text-xs text-muted-foreground font-normal">
                  v{selectedSkill.version}
                </span>
              </h2>
            </div>
            <div className="flex items-center gap-2 shrink-0">
              <button
                onClick={() => handleInstall("user")}
                disabled={installing}
                className="inline-flex items-center gap-1.5 h-8 px-3 rounded-lg text-sm font-medium border border-border hover:bg-muted transition-colors disabled:opacity-50"
              >
                {installing && installTarget === "user" ? (
                  <>
                    <Loader2 className="w-3.5 h-3.5 animate-spin" />
                    {t("skill.marketplace.installing")}
                  </>
                ) : installedNames.has(selectedSkill.name) ? (
                  canUpdateDetail ? (
                    t("skill.marketplace.updateToUser")
                  ) : (
                    t("skill.marketplace.reinstallToUser")
                  )
                ) : (
                  t("skill.marketplace.installToUser")
                )}
              </button>
              <button
                onClick={() => handleInstall("novel")}
                disabled={installing || !novelId}
                className="inline-flex items-center gap-1.5 h-8 px-3 rounded-lg text-sm font-medium bg-primary text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
              >
                {installing && installTarget === "novel" ? (
                  <>
                    <Loader2 className="w-3.5 h-3.5 animate-spin" />
                    {t("skill.marketplace.installing")}
                  </>
                ) : installedNames.has(selectedSkill.name) ? (
                  canUpdateDetail ? (
                    t("skill.marketplace.updateToNovel")
                  ) : (
                    t("skill.marketplace.reinstallToNovel")
                  )
                ) : (
                  t("skill.marketplace.installToNovel")
                )}
              </button>
              <button
                onClick={() => onOpenChange(false)}
                className="inline-flex items-center justify-center w-8 h-8 rounded-lg border border-border hover:bg-muted transition-colors text-muted-foreground hover:text-foreground"
                title={t("skill.marketplace.close")}
              >
                <X className="w-4 h-4" />
              </button>
            </div>
          </div>
        )}

        {phase === "confirm_overwrite" && selectedSkill && (
          <div className="flex items-center justify-between px-6 py-4 border-b shrink-0 gap-3">
            <div className="flex items-center gap-2 min-w-0">
              <button
                onClick={() => setPhase("detail")}
                className="inline-flex items-center justify-center w-8 h-8 rounded-lg border border-border hover:bg-muted transition-colors text-muted-foreground hover:text-foreground shrink-0"
                title={t("skill.marketplace.back")}
              >
                <ArrowLeft className="w-4 h-4" />
              </button>
              <h2 className="text-base font-semibold truncate">
                {t("skill.marketplace.overwriteTitle")}
              </h2>
            </div>
            <div className="flex items-center gap-2 shrink-0">
              <button
                onClick={() => setPhase("detail")}
                disabled={installing}
                className="h-8 px-3 rounded-lg text-sm border border-border hover:bg-muted transition-colors disabled:opacity-50"
              >
                {t("skill.marketplace.cancel")}
              </button>
              <button
                onClick={handleConfirmOverwrite}
                disabled={installing}
                className="inline-flex items-center gap-1.5 h-8 px-3 rounded-lg text-sm font-medium bg-destructive text-destructive-foreground hover:bg-destructive/90 transition-colors disabled:opacity-50"
              >
                {installing ? (
                  <>
                    <Loader2 className="w-3.5 h-3.5 animate-spin" />
                    {t("skill.marketplace.installing")}
                  </>
                ) : (
                  t("skill.marketplace.confirmOverwrite")
                )}
              </button>
              <button
                onClick={() => onOpenChange(false)}
                className="inline-flex items-center justify-center w-8 h-8 rounded-lg border border-border hover:bg-muted transition-colors text-muted-foreground hover:text-foreground"
                title={t("skill.marketplace.close")}
              >
                <X className="w-4 h-4" />
              </button>
            </div>
          </div>
        )}

        {/* Content area — switches by phase */}
        {phase === "browse" && (
          <div className="flex-1 min-h-0 flex flex-col">
            {/* Error bar */}
            {error && (
              <div className="mx-6 mt-3 px-3 py-2 text-xs text-destructive bg-danger-bg border border-danger-border rounded-md shrink-0">
                <div className="flex items-start gap-2">
                  <AlertTriangle className="w-4 h-4 shrink-0 mt-0.5" />
                  <div className="flex-1">
                    <div>{error.message}</div>
                    {error.code === "network" && (
                      <button
                        onClick={() => BrowserOpenURL(REPO_URL)}
                        className="mt-1 inline-flex items-center gap-1 text-primary hover:underline"
                      >
                        {t("skill.marketplace.repoLink")}
                        <ExternalLink className="w-3 h-3 opacity-60" />
                      </button>
                    )}
                  </div>
                  {canRetry && (
                    <button
                      onClick={handleRefresh}
                      className="shrink-0 px-2 py-0.5 rounded border border-destructive/40 hover:bg-destructive/10 transition-colors"
                    >
                      {t("skill.marketplace.retry")}
                    </button>
                  )}
                </div>
              </div>
            )}

            {/* Card grid */}
            <div className="flex-1 min-h-0 overflow-y-auto px-6 py-4">
              {loading ? (
                <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">
                  <Loader2 className="w-4 h-4 animate-spin mr-2" />
                  {t("skill.marketplace.loading")}
                </div>
              ) : items.length === 0 && !error ? (
                <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">
                  {t("skill.marketplace.empty")}
                </div>
              ) : (
                <div className="grid gap-4 grid-cols-[repeat(auto-fill,minmax(280px,1fr))]">
                  {items.map((sk) => {
                    const installed = installedNames.has(sk.name);
                    const installedVer = installedVersions.get(sk.name);
                    const canUpdate =
                      installed &&
                      installedVer !== undefined &&
                      sk.version > installedVer;
                    return (
                      <div
                        key={sk.name}
                        onClick={() => handleCardClick(sk)}
                        className={`group relative flex flex-col rounded-2xl p-5 transition-all duration-300 cursor-pointer select-none backdrop-blur-2xl border
                          ${
                            canUpdate
                              ? "bg-warning/60 border-warning-border/50 opacity-75 hover:opacity-100"
                              : installed
                                ? "bg-success/60 border-success-border/50 opacity-75 hover:opacity-100"
                                : "bg-card/80 border-white/15 hover:border-primary/20 hover:shadow-lg hover:-translate-y-0.5"
                          }`}
                      >
                        <div className="flex items-start justify-between mb-2 gap-2">
                          <h3 className="text-base font-semibold text-foreground truncate flex-1">
                            {sk.name}
                          </h3>
                          <span className="text-xs text-muted-foreground shrink-0">
                            v{sk.version}
                          </span>
                        </div>
                        <p className="text-[13px] text-muted-foreground leading-relaxed mb-3 line-clamp-4">
                          {sk.description}
                        </p>
                        <div className="flex items-center flex-wrap gap-1.5 text-[11px] text-muted-foreground/80 mt-auto">
                          {sk.category && (
                            <span className="inline-block px-2 py-0.5 rounded-full bg-muted/60 text-foreground/70 border border-border">
                              {sk.category}
                            </span>
                          )}
                          {sk.mode && (
                            <span
                              className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-md font-medium ${modeClass(sk.mode)}`}
                            >
                              {modeLabel(t, sk.mode)}
                            </span>
                          )}
                          {sk.author && (
                            <span className="inline-block px-1 py-0.5 truncate">
                              · {sk.author}
                            </span>
                          )}
                        </div>
                        {installed && installedVer !== undefined && (
                          <span
                            className={`absolute right-3 bottom-3 text-[10px] ${canUpdate ? "text-warning-foreground" : "text-success-foreground"}`}
                          >
                            {canUpdate
                              ? t("skill.marketplace.updatable", {
                                  version: sk.version,
                                })
                              : t("skill.marketplace.installed", {
                                  version: installedVer,
                                })}
                          </span>
                        )}
                      </div>
                    );
                  })}
                </div>
              )}
            </div>

            {/* Pagination bar */}
            <div className="flex items-center justify-between px-6 py-3 border-t shrink-0 text-xs text-muted-foreground">
              <div>
                {total > 0
                  ? t("skill.marketplace.pagination", {
                      total,
                      page,
                      totalPages,
                      count: total,
                    })
                  : ""}
              </div>
              <div className="flex items-center gap-2">
                <select
                  value={pageSize}
                  onChange={(e) => {
                    setPageSize(Number(e.target.value));
                    setPage(1);
                  }}
                  className="h-7 px-2 rounded-md border border-border bg-background text-xs"
                  title={t("skill.marketplace.pageSize", { size: pageSize })}
                >
                  {PAGE_SIZE_OPTIONS.map((s) => (
                    <option key={s} value={s}>
                      {s}
                    </option>
                  ))}
                </select>
                <button
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                  disabled={!canPrev}
                  className="h-7 px-2 rounded-md border border-border hover:bg-muted transition-colors disabled:opacity-40"
                >
                  ‹
                </button>
                <span className="text-xs">
                  {page} / {Math.max(1, totalPages)}
                </span>
                <button
                  onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                  disabled={!canNext}
                  className="h-7 px-2 rounded-md border border-border hover:bg-muted transition-colors disabled:opacity-40"
                >
                  ›
                </button>
              </div>
            </div>
          </div>
        )}

        {phase === "detail" && selectedSkill && (
          <div className="flex-1 min-h-0 overflow-y-auto px-6 py-4 space-y-3">
            {contentLoading ? (
              <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">
                <Loader2 className="w-4 h-4 animate-spin mr-2" />
                {t("skill.marketplace.contentLoading")}
              </div>
            ) : detailContentError ? (
              <div className="px-3 py-2 text-xs text-destructive bg-danger-bg border border-danger-border rounded-md">
                {detailContentError}
              </div>
            ) : (
              <>
                {/* frontmatter table */}
                {Object.keys(remoteSplit.meta).length > 0 && (
                  <table className="border bg-muted/20 w-full text-sm rounded-lg overflow-hidden">
                    <tbody>
                      {remoteSplit.meta.name && (
                        <tr className="border-b">
                          <td className="px-4 py-2 text-muted-foreground whitespace-nowrap w-20">
                            {t("skill.marketplace.fieldName")}
                          </td>
                          <td className="px-4 py-2 text-foreground font-semibold">
                            {remoteSplit.meta.name}
                          </td>
                        </tr>
                      )}
                      {remoteSplit.meta.description && (
                        <tr className="border-b">
                          <td className="px-4 py-2 text-muted-foreground whitespace-nowrap w-20">
                            {t("skill.marketplace.fieldDescription")}
                          </td>
                          <td className="px-4 py-2 text-foreground">
                            {remoteSplit.meta.description}
                          </td>
                        </tr>
                      )}
                      {remoteSplit.meta.category && (
                        <tr className="border-b">
                          <td className="px-4 py-2 text-muted-foreground whitespace-nowrap w-20">
                            {t("skill.marketplace.fieldCategory")}
                          </td>
                          <td className="px-4 py-2 text-foreground">
                            {remoteSplit.meta.category}
                          </td>
                        </tr>
                      )}
                      {remoteSplit.meta.mode && (
                        <tr className="border-b">
                          <td className="px-4 py-2 text-muted-foreground whitespace-nowrap w-20">
                            {t("skill.marketplace.fieldMode")}
                          </td>
                          <td className="px-4 py-2">
                            <span
                              className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-xs font-medium ${modeClass(remoteSplit.meta.mode)}`}
                            >
                              {modeLabel(t, remoteSplit.meta.mode)}
                            </span>
                          </td>
                        </tr>
                      )}
                      {remoteSplit.meta.author && (
                        <tr className="border-b">
                          <td className="px-4 py-2 text-muted-foreground whitespace-nowrap w-20">
                            {t("skill.marketplace.fieldAuthor")}
                          </td>
                          <td className="px-4 py-2 text-foreground">
                            {remoteSplit.meta.author}
                          </td>
                        </tr>
                      )}
                      {remoteSplit.meta.version && (
                        <tr>
                          <td className="px-4 py-2 text-muted-foreground whitespace-nowrap w-20">
                            {t("skill.marketplace.fieldVersion")}
                          </td>
                          <td className="px-4 py-2 text-foreground">
                            v{remoteSplit.meta.version}
                          </td>
                        </tr>
                      )}
                    </tbody>
                  </table>
                )}
                {/* markdown body */}
                <div className="rounded-lg border bg-muted/10 p-4">
                  <Markdown content={remoteSplit.body} />
                </div>
              </>
            )}
          </div>
        )}

        {phase === "confirm_overwrite" && selectedSkill && (
          <div className="flex-1 min-h-0 overflow-y-auto px-6 py-4 space-y-3">
            {/* overwrite warning bar */}
            <div className="px-3 py-2 text-xs text-warning-foreground bg-warning border border-warning-border rounded-md">
              {t("skill.marketplace.overwriteWarning", {
                target:
                  installTarget === "user"
                    ? t("skill.marketplace.targetUser")
                    : t("skill.marketplace.targetNovel"),
              })}
            </div>
            {/* compare two columns */}
            <div className="grid grid-cols-2 gap-4 min-h-0">
              <div className="border rounded-lg overflow-hidden flex flex-col min-h-0">
                <div className="px-3 py-2 text-xs font-medium bg-muted/60 border-b">
                  {t("skill.marketplace.localContent")}
                </div>
                <div className="flex-1 min-h-0 overflow-y-auto p-3 space-y-2">
                  {localContent ? (
                    <CompareView split={localSplit} t={t} />
                  ) : (
                    <div className="text-xs text-muted-foreground">
                      {t("skill.marketplace.noLocalContent")}
                    </div>
                  )}
                </div>
              </div>
              <div className="border rounded-lg overflow-hidden flex flex-col min-h-0">
                <div className="px-3 py-2 text-xs font-medium bg-muted/60 border-b">
                  {t("skill.marketplace.remoteContent")}
                </div>
                <div className="flex-1 min-h-0 overflow-y-auto p-3 space-y-2">
                  {remoteContentForConfirm ? (
                    <CompareView split={remoteForConfirmSplit} t={t} />
                  ) : contentError ? (
                    <div className="px-3 py-2 text-xs text-destructive bg-danger-bg border border-danger-border rounded-md">
                      {contentError}
                    </div>
                  ) : (
                    <div className="flex items-center gap-2 text-xs text-muted-foreground">
                      <Loader2 className="w-3 h-3 animate-spin" />
                      {t("skill.marketplace.contentLoading")}
                    </div>
                  )}
                </div>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

// Compact frontmatter + body view for the overwrite comparison columns.
function CompareView({
  split,
  t,
}: {
  split: { meta: Record<string, string>; body: string };
  t: TFunction;
}) {
  const { meta, body } = split;
  const rows: Array<[string, string]> = [];
  if (meta.name) rows.push([t("skill.marketplace.fieldName"), meta.name]);
  if (meta.description)
    rows.push([t("skill.marketplace.fieldDescription"), meta.description]);
  if (meta.category)
    rows.push([t("skill.marketplace.fieldCategory"), meta.category]);
  if (meta.mode) rows.push([t("skill.marketplace.fieldMode"), meta.mode]);
  if (meta.author) rows.push([t("skill.marketplace.fieldAuthor"), meta.author]);
  if (meta.version)
    rows.push([t("skill.marketplace.fieldVersion"), "v" + meta.version]);
  return (
    <>
      {rows.length > 0 && (
        <table className="w-full text-xs border bg-muted/20 rounded">
          <tbody>
            {rows.map(([k, v], i) => (
              <tr key={k} className={i < rows.length - 1 ? "border-b" : ""}>
                <td className="px-2 py-1 text-muted-foreground whitespace-nowrap w-16 align-top">
                  {k}
                </td>
                <td className="px-2 py-1 text-foreground break-words">{v}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {body && (
        <div className="rounded border bg-muted/10 p-2">
          <Markdown content={body} />
        </div>
      )}
    </>
  );
}

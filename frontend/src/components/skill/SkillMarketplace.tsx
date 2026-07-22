import { useCallback, useEffect, useMemo, useRef, useState } from "react";
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
import { useApp } from "@/hooks/useApp";
import type { remote } from "@/hooks/useApp";
import Markdown from "@/components/Markdown";
import { splitFrontmatter } from "@/components/content/types";
import { BrowserOpenURL } from "@/lib/wailsjs/runtime/runtime";
import { toastError } from "@/lib/utils";

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

export default function SkillMarketplace({
  open,
  onOpenChange,
  novelId,
  onInstalled,
}: Props) {
  const app = useApp();
  const { t } = useTranslation();

  const [phase, setPhase] = useState<Phase>("browse");
  const [query, setQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [items, setItems] = useState<remote.RemoteSkillMeta[]>([]);
  const [total, setTotal] = useState(0);
  const [totalPages, setTotalPages] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<{ code: string; message: string } | null>(
    null,
  );

  const [selectedSkill, setSelectedSkill] =
    useState<remote.RemoteSkillMeta | null>(null);
  const [remoteContent, setRemoteContent] = useState("");
  const [localContent, setLocalContent] = useState("");
  const [remoteContentForConfirm, setRemoteContentForConfirm] = useState("");
  const [contentLoading, setContentLoading] = useState(false);
  const [contentError, setContentError] = useState("");

  const [installTarget, setInstallTarget] = useState<InstallTarget>("user");
  const [installing, setInstalling] = useState(false);

  const [installedNames, setInstalledNames] = useState<Set<string>>(new Set());
  const [installedVersions, setInstalledVersions] = useState<
    Map<string, number>
  >(new Map());

  const canUpdateDetail = useMemo(() => {
    if (!selectedSkill) return false;
    const v = installedVersions.get(selectedSkill.name);
    return v !== undefined && selectedSkill.version > v;
  }, [selectedSkill, installedVersions]);

  const debounceTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Reset state when modal closes
  useEffect(() => {
    if (!open) {
      setPhase("browse");
      setSelectedSkill(null);
      setRemoteContent("");
      setLocalContent("");
      setRemoteContentForConfirm("");
      setInstallTarget("user");
      setQuery("");
      setDebouncedQuery("");
      setPage(1);
      setPageSize(DEFAULT_PAGE_SIZE);
      setError(null);
      setContentError("");
      setContentLoading(false);
      setInstalling(false);
      setItems([]);
      setTotal(0);
      setTotalPages(0);
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

  // Load installed skills index (for card visual differentiation)
  const loadInstalledIndex = useCallback(async () => {
    try {
      const list = await app.ListSkills({ novel_id: novelId });
      const nameSet = new Set<string>();
      const versionMap = new Map<string, number>();
      for (const s of list ?? []) {
        nameSet.add(s.name);
        const prev = versionMap.get(s.name) ?? 0;
        if (s.version > prev) versionMap.set(s.name, s.version);
      }
      setInstalledNames(nameSet);
      setInstalledVersions(versionMap);
    } catch (e) {
      console.error("Load installed skills failed", e);
    }
  }, [app, novelId]);

  // Load remote skills
  const loadRemote = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await app.ListRemoteSkills({
        page,
        size: pageSize,
        query: debouncedQuery,
      });
      const code = res?.err_code ?? "";
      if (code && code !== "ok") {
        const cls = classifyError(code, res?.err_msg ?? "", t);
        setError(cls);
        setItems([]);
        setTotal(0);
        setTotalPages(0);
      } else {
        const data = res?.data;
        setItems(data?.items ?? []);
        setTotal(data?.total ?? 0);
        setTotalPages(data?.total_pages ?? 0);
      }
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      setError({
        code: "other",
        message: t("skill.marketplace.errorOther", { message: msg }),
      });
      setItems([]);
      setTotal(0);
      setTotalPages(0);
    } finally {
      setLoading(false);
    }
  }, [app, page, pageSize, debouncedQuery, t]);

  // Initial load when modal opens
  useEffect(() => {
    if (open) {
      loadInstalledIndex();
      loadRemote();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  // Reload when page/pageSize/debouncedQuery changes
  useEffect(() => {
    if (open) loadRemote();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, pageSize, debouncedQuery]);

  // Load remote content when entering detail phase.
  // 返回 content 字符串，调用方直接用返回值，避免闭包捕获过期的 remoteContent state。
  const loadRemoteContent = useCallback(
    async (name: string): Promise<string> => {
      setContentLoading(true);
      setContentError("");
      setRemoteContent("");
      try {
        const res = await app.GetRemoteSkillContent(name);
        const code = res?.err_code ?? "";
        if (code && code !== "ok") {
          const cls = classifyError(code, res?.err_msg ?? "", t);
          setContentError(cls.message);
          return "";
        }
        const content = res?.data ?? "";
        setRemoteContent(content);
        return content;
      } catch (e: unknown) {
        const msg = e instanceof Error ? e.message : String(e);
        setContentError(t("skill.marketplace.errorOther", { message: msg }));
        return "";
      } finally {
        setContentLoading(false);
      }
    },
    [app, t],
  );

  // Click card → enter detail phase
  const handleCardClick = useCallback(
    (sk: remote.RemoteSkillMeta) => {
      setSelectedSkill(sk);
      setPhase("detail");
      setRemoteContent("");
      setLocalContent("");
      setRemoteContentForConfirm("");
      setContentError("");
      loadRemoteContent(sk.name);
    },
    [loadRemoteContent],
  );

  // GetContent probe for install target
  const probeLocal = useCallback(
    async (target: InstallTarget, name: string): Promise<string> => {
      if (target === "novel" && !novelId) return "";
      const path = pathForSource(target, name);
      try {
        const content = await app.GetContent(novelId, path);
        return content || "";
      } catch {
        return "";
      }
    },
    [app, novelId],
  );

  // Install skill
  const doInstall = useCallback(
    async (target: InstallTarget, name: string) => {
      setInstalling(true);
      try {
        const res = await app.InstallRemoteSkill({
          name,
          target,
          novel_id: novelId,
        });
        const code = res?.err_code ?? "";
        if (code && code !== "ok") {
          const cls = classifyError(code, res?.err_msg ?? "", t);
          toastError(cls.message);
          return;
        }
        // success — refresh local + remote, trigger callback, back to browse
        await loadInstalledIndex();
        await loadRemote();
        onInstalled?.();
        setPhase("browse");
        setSelectedSkill(null);
        setRemoteContent("");
        setLocalContent("");
        setRemoteContentForConfirm("");
      } catch (e: unknown) {
        const msg = e instanceof Error ? e.message : String(e);
        toastError(t("skill.marketplace.installFailed") + ": " + msg);
      } finally {
        setInstalling(false);
      }
    },
    [app, novelId, t, loadInstalledIndex, loadRemote, onInstalled],
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
        // 直接用 loadRemoteContent 返回值，避免闭包捕获过期的 remoteContent state；
        // 已有缓存时复用，否则拉取
        const remote =
          remoteContent || (await loadRemoteContent(selectedSkill.name));
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
      remoteContent,
      loadRemoteContent,
      doInstall,
    ],
  );

  // Confirm overwrite
  const handleConfirmOverwrite = useCallback(async () => {
    if (!selectedSkill) return;
    await doInstall(installTarget, selectedSkill.name);
  }, [selectedSkill, installTarget, doInstall]);

  // Refresh button
  const handleRefresh = useCallback(() => {
    loadRemote();
    loadInstalledIndex();
  }, [loadRemote, loadInstalledIndex]);

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
                  setRemoteContent("");
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
            ) : contentError ? (
              <div className="px-3 py-2 text-xs text-destructive bg-danger-bg border border-danger-border rounded-md">
                {contentError}
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

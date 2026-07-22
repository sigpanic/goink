import { useState, useEffect } from "react";
import {
  Folder,
  RefreshCw,
  GitFork,
  Languages,
  Download,
  Loader2,
  CheckCircle2,
  AlertCircle,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import {
  SaveGitConfig,
  GetVersion,
  CheckUpdate,
} from "@/lib/wailsjs/go/app/App";
import type { update as updateModels } from "@/lib/wailsjs/go/models";
import { useApp, type novel } from "@/hooks/useApp";
import UpdateDialog from "@/components/update/UpdateDialog";

export default function GeneralConfigTab() {
  const app = useApp();
  const { t, i18n } = useTranslation();
  const [dataDir, setDataDir] = useState("");
  const [novels, setNovels] = useState<novel.Novel[]>([]);
  const [selectedID, setSelectedID] = useState<number>(0);
  const [rebuilding, setRebuilding] = useState(false);
  const [gitName, setGitName] = useState("");
  const [gitEmail, setGitEmail] = useState("");
  const [gitSaving, setGitSaving] = useState(false);
  const [gitSaved, setGitSaved] = useState(false);
  const [gitError, setGitError] = useState<string | null>(null);
  const [appVersion, setAppVersion] = useState("");
  const [checking, setChecking] = useState(false);
  const [updateResult, setUpdateResult] =
    useState<updateModels.CheckResult | null>(null);
  const [updateError, setUpdateError] = useState<string | null>(null);
  const [showUpdateDialog, setShowUpdateDialog] = useState(false);

  useEffect(() => {
    app
      .GetAppConfig()
      .then((cfg) => {
        setDataDir((cfg?.data_dir as string) || "");
      })
      .catch(() => {});
    app
      .GetNovels()
      .then((list) => {
        setNovels(list || []);
      })
      .catch(() => {});
    app
      .GetSettings()
      .then((s) => {
        if (s?.last_novel_id) setSelectedID(s.last_novel_id);
        if (s?.git_name) setGitName(s.git_name);
        if (s?.git_email) setGitEmail(s.git_email);
      })
      .catch(() => {});
    GetVersion()
      .then((v) => setAppVersion(v || "dev"))
      .catch(() => {});
  }, [app]);

  async function handleSaveGit() {
    setGitSaving(true);
    setGitSaved(false);
    setGitError(null);
    try {
      await SaveGitConfig(gitName, gitEmail);
      setGitSaved(true);
      setTimeout(() => setGitSaved(false), 2000);
    } catch (err) {
      setGitError(
        err instanceof Error ? err.message : t("settings.saveFailed"),
      );
    } finally {
      setGitSaving(false);
    }
  }

  async function handleRebuild() {
    if (!selectedID) return;
    setRebuilding(true);
    try {
      await app.RebuildNovelIndex(selectedID);
    } catch (err) {
      console.error("Rebuild failed:", err);
    } finally {
      setRebuilding(false);
    }
  }

  async function handleCheckUpdate() {
    setChecking(true);
    setUpdateResult(null);
    setUpdateError(null);
    try {
      const result = await CheckUpdate(true);
      setUpdateResult(result);
      if (result?.hasUpdate) {
        setShowUpdateDialog(true);
      }
    } catch (err) {
      setUpdateError(
        err instanceof Error ? err.message : t("update.checkFailed"),
      );
    } finally {
      setChecking(false);
    }
  }

  return (
    <div className="flex-1 flex flex-col">
      <h3 className="text-sm font-medium mb-5">{t("settings.basicConfig")}</h3>

      <div className="space-y-2">
        <label className="text-xs font-medium text-muted-foreground flex items-center gap-1.5">
          <Folder className="w-3.5 h-3.5" />
          {t("settings.dataDir")}
        </label>
        <div className="flex items-center gap-2">
          <input
            value={dataDir}
            readOnly
            className="flex-1 h-8 rounded-md border bg-muted/50 px-3 text-xs font-mono focus:outline-none cursor-default"
          />
        </div>
      </div>

      <div className="mt-6 space-y-3">
        <label className="text-xs font-medium text-muted-foreground flex items-center gap-1.5">
          <GitFork className="w-3.5 h-3.5" />
          {t("settings.gitConfig")}
        </label>
        <p className="text-[11px] text-muted-foreground">
          {t("settings.gitConfigDesc")}
        </p>
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground w-12 shrink-0">
              {t("settings.nickname")}
            </span>
            <input
              value={gitName}
              onChange={(e) => setGitName(e.target.value)}
              placeholder="Goink"
              className="flex-1 h-8 rounded-md border bg-background px-3 text-xs focus:outline-none focus:ring-1 focus:ring-primary"
            />
          </div>
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground w-12 shrink-0">
              {t("settings.email")}
            </span>
            <input
              value={gitEmail}
              onChange={(e) => setGitEmail(e.target.value)}
              placeholder="goink@local"
              className="flex-1 h-8 rounded-md border bg-background px-3 text-xs focus:outline-none focus:ring-1 focus:ring-primary"
            />
          </div>
          <div className="flex items-center justify-between gap-2 pt-1">
            <div className="flex items-center gap-2">
              {gitError && (
                <span className="text-[11px] text-rose-500">{gitError}</span>
              )}
            </div>
            <button
              onClick={handleSaveGit}
              disabled={gitSaving}
              className="inline-flex items-center gap-1.5 h-8 px-4 rounded-md text-xs border hover:bg-muted transition-colors disabled:opacity-50"
            >
              {gitSaving
                ? t("common.saving")
                : gitSaved
                  ? t("common.saved")
                  : t("common.save")}
            </button>
          </div>
        </div>
      </div>

      <div className="mt-6 space-y-2">
        <label className="text-xs font-medium text-muted-foreground flex items-center gap-1.5">
          <Languages className="w-3.5 h-3.5" />
          {t("settings.language")}
        </label>
        <div className="inline-flex items-center gap-1 rounded-lg bg-muted/60 p-0.5">
          <button
            onClick={() => i18n.changeLanguage("zh-CN")}
            className={`h-7 px-3 rounded-md text-xs transition-colors ${
              i18n.language.startsWith("zh")
                ? "bg-background text-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground"
            }`}
          >
            中文
          </button>
          <button
            onClick={() => i18n.changeLanguage("en")}
            className={`h-7 px-3 rounded-md text-xs transition-colors ${
              i18n.language === "en"
                ? "bg-background text-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground"
            }`}
          >
            English
          </button>
        </div>
      </div>

      <div className="mt-6 space-y-2">
        <label className="text-xs font-medium text-muted-foreground">
          {t("settings.maintenance")}
        </label>
        <p className="text-[11px] text-muted-foreground">
          {t("settings.rebuildIndexDesc")}
        </p>
        <div className="flex items-center gap-2">
          <select
            value={selectedID}
            onChange={(e) => setSelectedID(Number(e.target.value))}
            className="h-8 rounded-md border bg-background px-2 text-xs focus:outline-none"
          >
            {novels.map((n) => (
              <option key={n.id} value={n.id}>
                {n.title}
              </option>
            ))}
          </select>
          <button
            onClick={handleRebuild}
            disabled={rebuilding || !selectedID}
            className="inline-flex items-center gap-1.5 h-8 px-3 rounded-md text-xs border hover:bg-muted transition-colors disabled:opacity-50"
          >
            <RefreshCw
              className={`w-3.5 h-3.5 ${rebuilding ? "animate-spin" : ""}`}
            />
            {rebuilding ? t("settings.rebuilding") : t("settings.rebuildIndex")}
          </button>
        </div>
      </div>

      <div className="mt-6 space-y-2">
        <label className="text-xs font-medium text-muted-foreground flex items-center gap-1.5">
          <Download className="w-3.5 h-3.5" />
          {t("update.versionAndUpdate")}
        </label>
        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground">
            {t("update.currentVersion")}
          </span>
          <span className="text-xs font-mono text-foreground">
            v{appVersion}
          </span>
          <button
            onClick={handleCheckUpdate}
            disabled={checking}
            className="inline-flex items-center gap-1.5 h-8 px-3 rounded-md text-xs border hover:bg-muted transition-colors disabled:opacity-50"
          >
            {checking ? (
              <Loader2 className="w-3.5 h-3.5 animate-spin" />
            ) : (
              <RefreshCw className="w-3.5 h-3.5" />
            )}
            {checking ? t("update.checking") : t("update.checkNow")}
          </button>
          {updateResult?.hasUpdate && (
            <span className="inline-flex items-center gap-1 text-xs text-primary">
              <Download className="w-3 h-3" />
              {t("update.versionLabel", {
                version: updateResult.latest.tag_name,
              })}
            </span>
          )}
          {updateResult && !updateResult.hasUpdate && (
            <span className="inline-flex items-center gap-1 text-xs text-tag-green-foreground">
              <CheckCircle2 className="w-3 h-3" />
              {t("update.upToDate")}
            </span>
          )}
          {updateError && (
            <span className="inline-flex items-center gap-1 text-xs text-rose-500">
              <AlertCircle className="w-3 h-3" />
              {updateError}
            </span>
          )}
        </div>
      </div>

      <UpdateDialog
        open={showUpdateDialog}
        result={updateResult}
        onClose={() => setShowUpdateDialog(false)}
      />
    </div>
  );
}

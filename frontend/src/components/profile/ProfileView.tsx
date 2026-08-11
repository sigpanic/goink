import { useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useApp } from "@/hooks/useApp";
import ContributionGrid from "./ContributionGrid";
import { PenLine, CalendarDays, Flame, User, Camera } from "lucide-react";
import { toErrorMessage } from "@/utils/error";
import { useWritingActivity } from "./useWritingActivity";
import { useWritingStats } from "./useWritingStats";
import { useProfileSettings } from "./useProfileSettings";
import { settingsKeys } from "@/lib/queryKeys";

export default function ProfileView() {
  // 5.7 commit 2 会删 useApp（SaveAvatar/SaveUserName 改 mutation 后清零）
  const app = useApp();
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  // 5.7 commit 1: 3 GET query 化（删 load 三件套 + useEffect + Promise.all）。
  // activity/stats/settings 各自独立 query，isLoading/isError 合并三态。
  // GET 错误由全局中间件接管（queryErrorToast.ts），组件不挂 toastError。
  const activityQuery = useWritingActivity(12);
  const statsQuery = useWritingStats();
  const settingsQuery = useProfileSettings();

  const isLoading =
    activityQuery.isLoading ||
    statsQuery.isLoading ||
    settingsQuery.isLoading;
  const isError =
    activityQuery.isError || statsQuery.isError || settingsQuery.isError;

  // activity 数组转 dict（绿格子按 date 取 words）
  const activity = useMemo(() => {
    const dict: Record<string, number> = {};
    for (const d of activityQuery.data ?? []) {
      dict[d.date] = d.words;
    }
    return dict;
  }, [activityQuery.data]);

  const stats = statsQuery.data ?? null;
  const settings = settingsQuery.data ?? null;

  // 组件内 UI state（规则 10，不进 store）
  const [avatarKey, setAvatarKey] = useState(0);
  const [editingName, setEditingName] = useState(false);
  const [nameDraft, setNameDraft] = useState("");
  const [avatarErrored, setAvatarErrored] = useState(false);
  const [avatarError, setAvatarError] = useState("");
  const [nameError, setNameError] = useState("");
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [currentYear] = useState(() => new Date().getFullYear());

  function handleAvatarClick() {
    fileInputRef.current?.click();
  }

  async function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    e.target.value = "";
    try {
      const buf = await file.arrayBuffer();
      await app.SaveAvatar(Array.from(new Uint8Array(buf)));
      setAvatarErrored(false);
      setAvatarKey((prev) => prev + 1);
      setAvatarError("");
    } catch (err) {
      setAvatarError(toErrorMessage(err));
    }
  }

  async function handleNameSave() {
    const name = nameDraft.trim();
    if (name && name !== settings?.user_name) {
      try {
        await app.SaveUserName(name);
        // 5.7 commit 1: invalidate settingsKeys.all 让 useProfileSettings refetch
        // 拿新 user_name（替代原 setSettings 局部更新；settingsKeys.all 与 chat
        // 共享缓存，一起刷新，chat 不读 user_name 不受影响）。
        queryClient.invalidateQueries({ queryKey: settingsKeys.all });
        setNameError("");
      } catch (err) {
        setNameError(toErrorMessage(err));
        return;
      }
    }
    setEditingName(false);
  }

  function startEditName() {
    setNameDraft(settings?.user_name ?? "");
    setEditingName(true);
  }

  if (isLoading) {
    return (
      <main className="flex-1 min-w-0 overflow-y-auto overscroll-contain bg-background">
        <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
          {t("profile.loading")}
        </div>
      </main>
    );
  }

  return (
    <main className="flex-1 min-w-0 overflow-y-auto overscroll-contain bg-background">
      <input
        ref={fileInputRef}
        type="file"
        accept="image/*"
        className="hidden"
        onChange={handleFileChange}
      />
      <div className="max-w-4xl mx-auto px-6 py-8 space-y-8">
        {isError && (
          <p className="text-xs text-destructive py-4">
            {t("profile.loadFailed")}
          </p>
        )}
        {/* 头像 + 问候 */}
        <div className="flex items-center gap-4">
          <div
            className="relative group flex-shrink-0 cursor-pointer select-none"
            onClick={handleAvatarClick}
          >
            {avatarErrored ? (
              <div className="w-14 h-14 rounded-full bg-muted bg-secondary flex items-center justify-center">
                <User className="w-7 h-7 text-muted-foreground" />
              </div>
            ) : (
              <img
                src={`/avatar?v=${avatarKey}`}
                alt=""
                onError={() => setAvatarErrored(true)}
                className="w-14 h-14 rounded-full object-cover"
              />
            )}
            <div className="absolute inset-0 rounded-full flex items-center justify-center bg-black/40 opacity-0 group-hover:opacity-100 transition-opacity">
              <Camera className="w-5 h-5 text-white" />
            </div>
          </div>
          {avatarError && (
            <p className="text-xs text-destructive">{avatarError}</p>
          )}
          <div>
            {editingName ? (
              <div>
                <input
                  autoFocus
                  value={nameDraft}
                  onChange={(e) => {
                    setNameDraft(e.target.value);
                    setNameError("");
                  }}
                  onBlur={handleNameSave}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") handleNameSave();
                    if (e.key === "Escape") setEditingName(false);
                  }}
                  className="text-lg font-semibold bg-transparent border-b border-primary outline-none text-foreground max-w-[200px]"
                />
                {nameError && (
                  <p className="text-xs text-destructive mt-0.5">{nameError}</p>
                )}
              </div>
            ) : (
              <h1
                onClick={startEditName}
                className={`text-lg font-semibold cursor-pointer hover:text-primary transition-colors select-none ${settings?.user_name ? "text-foreground" : "text-muted-foreground"}`}
              >
                {settings?.user_name || t("profile.noNickname")}
              </h1>
            )}
            <p className="text-xs text-muted-foreground mt-0.5">
              {t("profile.pastYearStats", {
                count: Object.keys(activity).length,
              })}
            </p>
          </div>
        </div>

        {/* 统计卡片 */}
        <div className="grid grid-cols-2 gap-3">
          <StatCard
            icon={PenLine}
            label={t("profile.totalWords")}
            value={(stats?.total_words ?? 0).toLocaleString()}
          />
          <StatCard
            icon={CalendarDays}
            label={t("profile.writingDays")}
            value={`${stats?.total_days_active ?? 0}`}
          />
          <StatCard
            icon={Flame}
            label={t("profile.streakDays")}
            value={`${stats?.current_streak ?? 0} ${t("profile.day")}`}
          />
          <StatCard
            icon={Flame}
            label={t("profile.longestStreak")}
            value={`${stats?.longest_streak ?? 0} ${t("profile.day")}`}
          />
        </div>

        {/* 作品/章节概览 */}
        <div className="flex gap-6 text-xs text-muted-foreground">
          <span>
            {t("profile.worksCount", { count: stats?.total_novels ?? 0 })}
          </span>
          <span>
            {t("profile.chaptersCount", { count: stats?.total_chapters ?? 0 })}
          </span>
        </div>

        {/* 绿格子 */}
        <section>
          <h2 className="text-sm font-medium text-foreground mb-4">
            {t("profile.yearCalendar", { year: currentYear })}
          </h2>
          <div className="overflow-x-auto">
            <ContributionGrid data={activity} />
          </div>
        </section>

        {Object.keys(activity).length === 0 && (
          <div className="text-center py-12">
            <PenLine className="w-10 h-10 mx-auto text-muted-foreground mb-3" />
            <p className="text-sm text-muted-foreground">
              {t("profile.noWritingRecord")}
            </p>
          </div>
        )}
      </div>
    </main>
  );
}

function StatCard({
  icon: Icon,
  label,
  value,
}: {
  icon: any;
  label: string;
  value: string;
}) {
  return (
    <div className="rounded-lg border bg-card px-4 py-3 space-y-1">
      <div className="flex items-center gap-1.5 text-muted-foreground">
        <Icon className="w-3.5 h-3.5" />
        <span className="text-[11px]">{label}</span>
      </div>
      <p className="text-lg font-semibold text-foreground">{value}</p>
    </div>
  );
}

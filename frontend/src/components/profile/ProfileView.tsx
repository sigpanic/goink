import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import ContributionGrid from "./ContributionGrid";
import { PenLine, CalendarDays, Flame } from "lucide-react";
import { useWritingActivity } from "./useWritingActivity";
import type { WritingActivity } from "./useWritingActivity";
import { useWritingStats } from "./useWritingStats";
import { useProfileSettings } from "./useProfileSettings";
import ProfileHeader from "./ProfileHeader";
import AnnualReportEntry from "./AnnualReportEntry";
import type { useReportSeason } from "@/components/annual-report/useReportSeason";
import reportStyles from "@/components/annual-report/annual-report.module.css";

export default function ProfileView({
  reportSeason,
  onOpenAnnualReport,
}: {
  reportSeason: ReturnType<typeof useReportSeason>;
  onOpenAnnualReport: () => void;
}) {
  const { t } = useTranslation();

  // 5.7 commit 1: 3 GET query 化（删 load 三件套 + useEffect + Promise.all）。
  // activity/stats/settings 各自独立 query，isLoading/isError 合并三态。
  // GET 错误由全局中间件接管（queryErrorToast.ts），组件不挂 toastError。
  const activityQuery = useWritingActivity(12);
  const statsQuery = useWritingStats();
  const settingsQuery = useProfileSettings();

  const isLoading =
    activityQuery.isLoading || statsQuery.isLoading || settingsQuery.isLoading;
  const isError =
    activityQuery.isError || statsQuery.isError || settingsQuery.isError;

  // activity 数组转 dict（绿格子按 date 取 words）
  const activity = useMemo(() => {
    const dict: Record<string, WritingActivity> = {};
    for (const d of activityQuery.data ?? []) {
      dict[d.date] = d;
    }
    return dict;
  }, [activityQuery.data]);

  const stats = statsQuery.data ?? null;
  const settings = settingsQuery.data ?? null;

  const [currentYear] = useState(() => new Date().getFullYear());

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
    <main
      className={`${reportStyles["profile-view"]} flex-1 min-w-0 overflow-y-auto overscroll-contain bg-background`}
    >
      <div className="max-w-4xl mx-auto px-6 py-8 space-y-8">
        {isError && (
          <p className="text-xs text-destructive py-4">
            {t("profile.loadFailed")}
          </p>
        )}
        <ProfileHeader
          userName={settings?.user_name ?? ""}
          activityDays={Object.keys(activity).length}
        />

        <AnnualReportEntry season={reportSeason} onOpen={onOpenAnnualReport} />

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
            {t("profile.chaptersCount", {
              count: stats?.total_chapters ?? 0,
            })}
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

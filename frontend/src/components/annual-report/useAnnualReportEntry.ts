import { useCallback, useEffect, useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { MarkAnnualReportPrompted } from "@/lib/wailsjs/go/app/App";
import { settingsKeys } from "@/lib/queryKeys";
import { useProfileSettings } from "@/components/profile/useProfileSettings";
import { toErrorMessage } from "@/utils/error";
import { toastError } from "@/utils/toast";
import { useReportSeason } from "./useReportSeason";

export function useAnnualReportEntry(blocked: boolean) {
  const season = useReportSeason();
  const settings = useProfileSettings();
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  const [reportOpen, setReportOpen] = useState(false);
  const [promptYear, setPromptYear] = useState<number | null>(null);
  const promptedYear = useRef(0);
  const { mutate } = useMutation({
    mutationFn: (year: number) => MarkAnnualReportPrompted(year),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: settingsKeys.all });
    },
    onError: (error) => {
      toastError(toErrorMessage(error, t("annualReport.reminderSaveFailed")));
    },
  });
  const markPrompted = useCallback(
    (year: number) => {
      if (promptedYear.current >= year) return;
      // 本次运行先去重；保存失败时仍允许查看，下一次启动会重新提醒。
      promptedYear.current = year;
      mutate(year);
    },
    [mutate],
  );

  useEffect(() => {
    if (
      !season.available ||
      blocked ||
      reportOpen ||
      !settings.isSuccess ||
      (settings.data.annual_report_prompt_year ?? 0) >= season.reportYear ||
      promptedYear.current >= season.reportYear
    )
      return;
    setPromptYear(season.reportYear);
    markPrompted(season.reportYear);
  }, [
    season.available,
    season.reportYear,
    blocked,
    reportOpen,
    settings.isSuccess,
    settings.data,
    markPrompted,
  ]);

  function openReport() {
    if (!season.available) return;
    setPromptYear(null);
    setReportOpen(true);
    if (
      settings.isSuccess &&
      (settings.data.annual_report_prompt_year ?? 0) < season.reportYear
    )
      markPrompted(season.reportYear);
  }

  return {
    season,
    reportOpen: reportOpen && season.available,
    promptOpen:
      promptYear === season.reportYear &&
      season.available &&
      !blocked &&
      !reportOpen,
    openReport,
    closeReport: () => setReportOpen(false),
    dismissPrompt: () => setPromptYear(null),
  };
}

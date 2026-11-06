import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { GetAnnualReport } from "@/lib/wailsjs/go/app/App";
import type { report } from "@/lib/wailsjs/go/models";
import { annualReportKeys } from "@/lib/queryKeys";
import { unwrapResult } from "@/utils/wailsResult";

export type AnnualReport = report.AnnualReport;

export function useAnnualReport(year: number) {
  return useQuery({
    queryKey: annualReportKeys.detail(year),
    queryFn: async () => unwrapResult(await GetAnnualReport(year)),
    placeholderData: keepPreviousData,
  });
}

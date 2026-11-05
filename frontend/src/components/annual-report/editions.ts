import type { ComponentType } from "react";
import type { AnnualReportEditionProps } from "./types";
import Report2026 from "./editions/2026/Report2026";

export const reportEditions: Partial<
  Record<number, ComponentType<AnnualReportEditionProps>>
> = {
  2026: Report2026,
};

export const DefaultReportEdition = Report2026;

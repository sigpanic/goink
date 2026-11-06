import { useEffect, useState } from "react";
import { Environment } from "@/lib/wailsjs/runtime/runtime";

export function isAnnualReportAvailable(
  now: Date,
  development = import.meta.env.DEV,
): boolean {
  if (development) return true;
  if (now < new Date(2026, 11, 29)) return false;
  return (
    (now.getMonth() === 11 && now.getDate() >= 29) ||
    (now.getMonth() === 0 && now.getDate() <= 15)
  );
}

export function defaultReportYear(now: Date): number {
  return now.getFullYear() - (now.getMonth() === 0 ? 1 : 0);
}

export function useReportSeason() {
  const [now, setNow] = useState(() => new Date());
  const [wailsDevelopment, setWailsDevelopment] = useState(false);
  useEffect(() => {
    if (import.meta.env.DEV) return;
    let active = true;
    void Promise.resolve()
      .then(() => Environment())
      .then((environment) => {
        if (active) setWailsDevelopment(environment.buildType === "dev");
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, []);
  useEffect(() => {
    const midnight = new Date(
      now.getFullYear(),
      now.getMonth(),
      now.getDate() + 1,
    );
    const timer = setTimeout(
      () => setNow(new Date()),
      midnight.getTime() - now.getTime(),
    );
    return () => clearTimeout(timer);
  }, [now]);
  return {
    available: isAnnualReportAvailable(
      now,
      import.meta.env.DEV || wailsDevelopment,
    ),
    reportYear: defaultReportYear(now),
  };
}

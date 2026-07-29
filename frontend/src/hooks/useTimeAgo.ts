import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

// useTimeAgo 封装"相对时间"渲染：维护一个每分钟刷新的 now 时间戳，
// 返回 timeAgo(iso) 把 ISO 时间字符串转成"刚刚 / x 分钟前 / x 小时前 / ..."。
//
// enabled 控制定时器是否运行：面板/列表可见时传 true 自动刷新相对时间，
// 不可见时传 false 停掉定时器，避免无意义重渲染。
// 复用点：SessionHistory(open)、RecentSessions、GitHistoryList。
//
// i18n key 统一用 time.* 命名空间（原 chat.*/git.* 里的相对时间 key 已迁出）。
export function useTimeAgo(enabled = true) {
  const { t } = useTranslation();
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    if (!enabled) return;
    const timer = setInterval(() => setNow(Date.now()), 60_000);
    return () => clearInterval(timer);
  }, [enabled]);

  return useCallback(
    (iso: string) => {
      const diff = now - new Date(iso).getTime();
      const min = Math.floor(diff / 60000);
      if (min < 1) return t("time.justNow");
      if (min < 60) return t("time.minutesAgo", { count: min });
      const hour = Math.floor(min / 60);
      if (hour < 24) return t("time.hoursAgo", { count: hour });
      const day = Math.floor(hour / 24);
      if (day < 30) return t("time.daysAgo", { count: day });
      return t("time.monthsAgo", { count: Math.floor(day / 30) });
    },
    [now, t],
  );
}

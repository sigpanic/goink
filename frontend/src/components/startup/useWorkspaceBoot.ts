import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { GetSettings } from "@/lib/wailsjs/go/app/App";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";

/**
 * 主界面就绪前的最后一步：读取"上次打开的书籍"。
 *
 * 以 phase/version 为输入而不是只看 phase：每次启动状态变化（含重试）都要重新
 * 取一次；且取完之前不放行主界面，否则会先闪一下 novelId=0 的书架。
 */
export function useWorkspaceBoot(phase: string | null, version: number | null) {
  const { t } = useTranslation();
  const [novelId, setNovelId] = useState(0);
  const [readyVersion, setReadyVersion] = useState<number | null>(null);

  useEffect(() => {
    if (phase !== "ready" || version === null) return;

    let active = true;
    GetSettings()
      .then((settings) => {
        if (active) setNovelId(settings?.last_novel_id ?? 0);
      })
      .catch((error) => {
        if (active) {
          toastError(toErrorMessage(error, t("chat.settingsLoadFailed")));
        }
      })
      .finally(() => {
        if (active) setReadyVersion(version);
      });
    return () => {
      active = false;
    };
  }, [phase, version, t]);

  return {
    novelId,
    ready: phase === "ready" && readyVersion === version,
  };
}

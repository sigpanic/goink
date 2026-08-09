import type { Dispatch, SetStateAction } from "react";
import { useTranslation } from "react-i18next";
import {
  WindowMinimise,
  WindowToggleMaximise,
  Quit,
} from "@/lib/wailsjs/runtime/runtime";

interface Props {
  platformOS: string;
  isMaximised: boolean;
  setIsMaximised: Dispatch<SetStateAction<boolean>>;
}

const winBtn =
  "w-12 h-full flex items-center justify-center cursor-pointer text-foreground/80 hover:text-foreground hover:bg-black/25 hover:shadow-md transition-all";
const closeBtn =
  "w-12 h-full flex items-center justify-center cursor-pointer text-foreground/80 hover:text-destructive-foreground hover:bg-destructive transition-colors";

// 非 macOS（darwin）显示自定义窗口按钮；macOS 走原生红绿灯。
// 从 WorkspaceView 抽出，header 双击最大化仍留在 WorkspaceView。
export default function WindowControls({
  platformOS,
  isMaximised,
  setIsMaximised,
}: Props) {
  const { t } = useTranslation();

  if (platformOS === "darwin") return null;

  return (
    <>
      <button
        onClick={WindowMinimise}
        className={winBtn}
        title={t("workspace.minimize")}
      >
        <svg width="12" height="12" viewBox="0 0 12 12">
          <path
            d="M2.5 6h7"
            stroke="currentColor"
            strokeWidth="1.1"
            strokeLinecap="round"
          />
        </svg>
      </button>
      <button
        onClick={() => {
          WindowToggleMaximise();
          setIsMaximised((prev) => !prev);
        }}
        className={winBtn}
        title={isMaximised ? t("workspace.restore") : t("workspace.maximize")}
      >
        {isMaximised ? (
          <svg width="12" height="12" viewBox="0 0 12 12">
            <rect
              x="4"
              y="1.5"
              width="6.5"
              height="6.5"
              rx="1"
              fill="none"
              stroke="currentColor"
              strokeWidth=".9"
            />
            <rect
              x="1.5"
              y="2.5"
              width="6.5"
              height="6.5"
              rx="1"
              fill="var(--color-sidebar)"
              stroke="currentColor"
              strokeWidth=".9"
            />
          </svg>
        ) : (
          <svg width="12" height="12" viewBox="0 0 12 12">
            <rect
              x="1.5"
              y="1.5"
              width="9"
              height="9"
              stroke="currentColor"
              strokeWidth=".9"
              rx=".5"
              fill="none"
            />
          </svg>
        )}
      </button>
      <button onClick={Quit} className={closeBtn} title={t("workspace.close")}>
        <svg width="12" height="12" viewBox="0 0 12 12">
          <path
            d="M2.5 2.5l7 7M9.5 2.5l-7 7"
            stroke="currentColor"
            strokeWidth="1"
            strokeLinecap="round"
          />
        </svg>
      </button>
    </>
  );
}

import { useState } from "react";
import { Settings, Cpu } from "lucide-react";
import { useTranslation } from "react-i18next";
import ModelConfigTab from "./ModelConfigTab";
import GeneralConfigTab from "./GeneralConfigTab";

type Tab = "general" | "model";

interface Props {
  open: boolean;
  onClose: () => void;
  initialTab?: Tab;
}

export default function SettingsDialog({
  open,
  onClose,
  initialTab = "model",
}: Props) {
  const { t } = useTranslation();
  const [activeTab, setActiveTab] = useState<Tab>(initialTab);

  if (!open) return null;

  const tabs: { id: Tab; label: string; icon: React.ReactNode }[] = [
    {
      id: "general",
      label: t("settings.general"),
      icon: <Settings className="w-4 h-4" />,
    },
    {
      id: "model",
      label: t("settings.modelConfig"),
      icon: <Cpu className="w-4 h-4" />,
    },
  ];

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      {/* 遮罩 */}
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />

      {/* 弹窗 */}
      <div className="relative bg-background rounded-xl shadow-2xl border flex w-[880px] h-[700px] max-w-[95vw] max-h-[90vh]">
        {/* 左侧导航 */}
        <nav className="w-[160px] border-r py-4 px-2 flex flex-col gap-1 shrink-0">
          <div className="text-sm font-medium px-3 pb-3 text-foreground">
            {t("settings.title")}
          </div>
          {tabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`flex items-center gap-2 px-3 py-2 rounded-lg text-sm transition-colors w-full text-left ${
                activeTab === tab.id
                  ? "bg-primary/10 text-primary font-medium"
                  : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
              }`}
            >
              {tab.icon}
              {tab.label}
            </button>
          ))}
        </nav>

        {/* 右侧内容区 */}
        <div className="flex-1 p-5 flex flex-col min-w-0">
          {/* 关闭按钮 */}
          <button
            onClick={onClose}
            className="absolute top-3 right-3 w-7 h-7 flex items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
          >
            ✕
          </button>

          {activeTab === "model" ? (
            <ModelConfigTab />
          ) : (
            <GeneralConfigTab />
          )}
        </div>
      </div>
    </div>
  );
}

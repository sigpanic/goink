import { useState } from "react";
import type { ReactNode } from "react";
import { BookOpen, Palette } from "lucide-react";
import { useTranslation } from "react-i18next";
import StyleView from "@/components/style/StyleView";
import PatternExtractView from "@/components/pattern/PatternExtractView";

interface Props {
  novelId: number;
  focusSampleId?: number | null;
  onFocusSampleHandled?: () => void;
}

type Tab = "style" | "pattern";

const tabs: { id: Tab; labelKey: string; icon: ReactNode }[] = [
  {
    id: "style",
    labelKey: "extract.styleSamples",
    icon: <Palette className="w-4 h-4" />,
  },
  {
    id: "pattern",
    labelKey: "extract.patternExtract",
    icon: <BookOpen className="w-4 h-4" />,
  },
];

export default function ExtractWorkspaceView({
  novelId,
  focusSampleId,
  onFocusSampleHandled,
}: Props) {
  const { t } = useTranslation();
  const [selectedTab, setSelectedTab] = useState<Tab>("style");
  const activeTab = focusSampleId ? "style" : selectedTab;

  const handleFocusSampleHandled = () => {
    setSelectedTab("style");
    onFocusSampleHandled?.();
  };

  return (
    <div className="flex-1 flex flex-col min-h-0 bg-background">
      <div className="px-6 pt-4 border-b shrink-0">
        <div className="inline-flex items-center gap-1 rounded-lg bg-muted/60 p-1">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setSelectedTab(tab.id)}
              className={`inline-flex items-center gap-1.5 h-8 px-3 rounded-md text-sm transition-colors ${
                activeTab === tab.id
                  ? "bg-background text-foreground shadow-sm"
                  : "text-muted-foreground hover:text-foreground hover:bg-background/60"
              }`}
            >
              {tab.icon}
              {t(tab.labelKey)}
            </button>
          ))}
        </div>
      </div>

      <div
        className={
          activeTab === "style" ? "flex-1 flex flex-col min-h-0" : "hidden"
        }
      >
        <StyleView
          focusId={activeTab === "style" ? focusSampleId : null}
          onFocusHandled={handleFocusSampleHandled}
          embedded
          novelId={novelId}
        />
      </div>
      <div
        className={
          activeTab === "pattern" ? "flex-1 flex flex-col min-h-0" : "hidden"
        }
      >
        <PatternExtractView currentNovelId={novelId} />
      </div>
    </div>
  );
}

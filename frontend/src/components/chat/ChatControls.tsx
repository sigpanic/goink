import type { llm } from "@/hooks/useApp";
import { useTranslation } from "react-i18next";
import PopSelect from "./PopSelect";
import ContextRing from "./ContextRing";
import type { UsageInfo } from "./ContextRing";

interface Props {
  models: llm.AvailableModel[];
  selectedKey: string;
  onSelectModel: (key: string) => void;
  onRefreshModels?: () => void;
  reasoningEffort: string;
  onSelectEffort: (effort: string) => void;
  approvalMode: "manual" | "auto";
  onToggleApproval: () => void;
  onConfigModel: () => void;
  usage: UsageInfo | null;
  onCompress?: () => void;
  isTurnRunning?: boolean;
  isCompressing?: boolean;
}

export default function ChatControls({
  models,
  selectedKey,
  onSelectModel,
  onRefreshModels,
  reasoningEffort,
  onSelectEffort,
  approvalMode,
  onToggleApproval,
  onConfigModel,
  usage,
  onCompress,
  isTurnRunning,
  isCompressing,
}: Props) {
  const { t } = useTranslation();
  const selected = models.find((m) => m.Key === selectedKey);
  const supportsReasoning =
    selected?.ReasoningLevels && selected.ReasoningLevels.length > 0;

  const modelOptions = models.map((m) => ({
    value: m.Key,
    label: m.ModelName,
  }));
  const reasoningOptions = supportsReasoning
    ? selected.ReasoningLevels.map((level) => ({
        value: level,
        label:
          level === "high" ? t("chat.highReasoning") : t("chat.maxReasoning"),
      }))
    : [];

  return (
    <div className="flex items-center gap-1.5 px-4 py-2 text-xs shrink-0 select-none">
      <PopSelect
        value={selectedKey}
        options={modelOptions}
        onChange={onSelectModel}
        onOpen={onRefreshModels}
        footerAction={{
          label: t("chat.configureModel"),
          onClick: onConfigModel,
        }}
      />

      {supportsReasoning && (
        <PopSelect
          value={reasoningEffort}
          options={reasoningOptions}
          onChange={onSelectEffort}
          minWidth="80px"
        />
      )}

      <div className="flex-1" />

      <button
        onClick={onToggleApproval}
        className={`h-[30px] rounded-lg border px-2.5 text-xs transition-colors shrink-0 ${
          approvalMode === "auto"
            ? "bg-primary/10 text-primary border-primary/30"
            : "text-muted-foreground"
        }`}
      >
        {t("chat.auto")}
      </button>

      <ContextRing
        usage={usage}
        onCompress={onCompress}
        isTurnRunning={isTurnRunning}
        isCompressing={isCompressing}
      />
    </div>
  );
}

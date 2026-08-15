import { useTranslation } from "react-i18next";
import PopSelect from "./PopSelect";
import ContextRing from "./ContextRing";
import type { UsageInfo } from "./ContextRing";
import { useModels } from "@/components/settings/useModels";
import { useChatStore } from "./useChatStore";

interface Props {
  onSelectModel: (key: string) => void;
  onRefreshModels?: () => void;
  onSelectEffort: (effort: string) => void;
  onToggleApproval: () => void;
  onConfigModel: () => void;
  usage: UsageInfo | null;
  onCompress?: () => void;
  isTurnRunning?: boolean;
  isCompressing?: boolean;
}

// ChatControls: 模型/推理/审批控件。
// models 列表走 useModels query 订阅；selectedModel/reasoningEffort/approvalMode
// 从 useChatStore 订阅（跨组件共享，废弃拼接 key）。回调仍由 props 传入（mutation commit 4 迁）。
export default function ChatControls({
  onSelectModel,
  onRefreshModels,
  onSelectEffort,
  onToggleApproval,
  onConfigModel,
  usage,
  onCompress,
  isTurnRunning,
  isCompressing,
}: Props) {
  const { t } = useTranslation();
  const modelsQuery = useModels();
  const models = modelsQuery.data ?? [];
  const selectedModel = useChatStore((s) => s.selectedModel);
  const reasoningEffort = useChatStore((s) => s.reasoningEffort);
  const approvalMode = useChatStore((s) => s.approvalMode);

  const selectedKey = selectedModel?.Key ?? "";
  const supportsReasoning =
    !!selectedModel?.ReasoningLevels &&
    selectedModel.ReasoningLevels.length > 0;

  const modelOptions = models.map((m) => ({
    value: m.Key,
    label: m.ModelName,
  }));
  const reasoningOptions = supportsReasoning
    ? selectedModel!.ReasoningLevels.map((level) => ({
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

import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { app, type adapter, type llm } from "@/lib/wailsjs/go/models";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";
import { useChatStore } from "./useChatStore";
import { useSetSelectedModel } from "./useSetSelectedModel";

interface Options {
  sessionId: string;
  models: llm.AvailableModel[] | undefined;
  defaultSelection: adapter.ModelSelection | undefined;
  session: app.SessionDetail | undefined;
  enabled: boolean;
}

function selectionFor(
  model: llm.AvailableModel,
  effort: string,
): adapter.ModelSelection {
  return {
    adapter_name: model.AdapterName,
    provider_name: model.ProviderName,
    model_id: model.model.id,
    reasoning_effort: effort,
  };
}

// 草稿按会话隔离，保存完成前不被旧查询结果覆盖；失败时保留用户选择供重试。
export function useChatModelSelection({
  sessionId,
  models,
  defaultSelection,
  session,
  enabled,
}: Options) {
  const { t } = useTranslation();
  const { mutateAsync } = useSetSelectedModel();
  const [drafts, setDrafts] = useState<Record<string, adapter.ModelSelection>>(
    {},
  );
  const saved = sessionId
    ? session?.session_id === sessionId
      ? session.model_selection
      : undefined
    : defaultSelection;
  const choice = drafts[sessionId] ?? saved;
  const matched = models?.find(
    (m) =>
      m.AdapterName === choice?.adapter_name &&
      m.ProviderName === choice?.provider_name &&
      m.model.id === choice?.model_id,
  );
  const selectedModel =
    enabled && choice ? (matched ?? models?.[0] ?? null) : null;
  const levels = selectedModel?.model.reasoning_levels;
  const reasoningEffort =
    matched && levels?.includes(choice?.reasoning_effort ?? "")
      ? choice!.reasoning_effort
      : (levels?.[0] ?? "");
  const modelSelection = useMemo(
    () => (selectedModel ? selectionFor(selectedModel, reasoningEffort) : null),
    [selectedModel, reasoningEffort],
  );

  useEffect(() => {
    useChatStore.setState({ selectedModel, reasoningEffort });
  }, [selectedModel, reasoningEffort]);

  const persist = useCallback(
    (next: adapter.ModelSelection, errorMessage: string) => {
      const scope = sessionId;
      setDrafts((prev) => ({ ...prev, [scope]: next }));
      void mutateAsync(
        new app.SetSelectedModelInput({
          session_id: scope,
          model_selection: next,
        }),
      )
        .then(() => {
          setDrafts((prev) => {
            if (prev[scope] !== next) return prev;
            const remaining = { ...prev };
            delete remaining[scope];
            return remaining;
          });
        })
        .catch((err) => toastError(toErrorMessage(err, errorMessage)));
    },
    [sessionId, mutateAsync],
  );

  const handleSelectModel = useCallback(
    (key: string) => {
      if (!enabled) return;
      const model = models?.find((m) => m.Key === key);
      if (!model) return;
      persist(
        selectionFor(model, model.model.reasoning_levels?.[0] ?? ""),
        t("chat.setSelectedModelFailed"),
      );
    },
    [enabled, models, persist, t],
  );

  const handleSelectEffort = useCallback(
    (effort: string) => {
      if (
        !selectedModel ||
        !selectedModel.model.reasoning_levels?.includes(effort)
      )
        return;
      persist(
        selectionFor(selectedModel, effort),
        t("chat.setReasoningEffortFailed"),
      );
    },
    [selectedModel, persist, t],
  );

  return {
    selectedModel,
    modelSelection,
    handleSelectModel,
    handleSelectEffort,
  };
}

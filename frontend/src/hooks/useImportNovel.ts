import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import type { imp } from "@/lib/wailsjs/go/models";
import {
  ImportNovel,
  PickAndImportNovel,
  ImportWithLLM,
} from "@/lib/wailsjs/go/app/App";
import { EventsOn } from "@/lib/wailsjs/runtime/runtime";
import { toErrorMessage } from "@/utils/error";
import { splitModelKey } from "@/utils/modelKey";
import { useModels } from "@/components/settings/useModels";
import { useSettings } from "@/components/settings/useSettings";

export type ImportProgressStage =
  | "idle"
  | "select_file"
  | "parse"
  | "create_novel"
  | "write_chapters"
  | "commit"
  | "done"
  | "error"
  | "needs_llm"
  | "analyzing";

export interface ImportProgressState {
  stage: ImportProgressStage;
  message: string;
  current: number;
  total: number;
  percent: number;
  novel_id?: number;
}

const INITIAL_IMPORT_PROGRESS: ImportProgressState = {
  stage: "idle",
  message: "",
  current: 0,
  total: 0,
  percent: 0,
};

interface UseImportNovelOptions {
  onImported: (result: imp.ImportResult) => Promise<void>;
}

export function useImportNovel({ onImported }: UseImportNovelOptions) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [progress, setProgress] = useState<ImportProgressState>({
    ...INITIAL_IMPORT_PROGRESS,
    message: t("novel.importPreparing2"),
  });
  const [error, setError] = useState("");
  const [skippedCount, setskipped_count] = useState(0);
  const [skippedChapters, setskipped_chapters] = useState<
    { title: string; reason: string }[]
  >([]);

  // LLM 兜底相关状态
  const [filePath, setFilePath] = useState("");
  // modelKey 是可变 state（用户可手动切换模型），需本地一份；
  // models 列表只读，直接用 useModels query data，不再同步到本地 state。
  const [modelKey, setModelKey] = useState("");

  // 5.9: 模型列表 + 当前选中模型走 query hook（useModels 5.1 / useSettings 5.8），
  // 替代原 useEffect 命令式 app.GetModels + app.GetSettings。删 app 参数后不再依赖 useApp。
  const { data: modelsData = [] } = useModels();
  const { data: settingsData } = useSettings();

  useEffect(() => {
    const unsubscribe = EventsOn(
      "import:progress",
      (data: ImportProgressState) => {
        setProgress({
          stage: data.stage,
          message: data.message,
          current: data.current ?? 0,
          total: data.total ?? 0,
          percent: data.percent ?? 0,
          novel_id: data.novel_id,
        });
        if (data.stage === "error") {
          setError(data.message);
        }
      },
    );
    return unsubscribe;
  }, []);

  // modelKey 选中态：query data ready 后从 settings 回填。
  // 原命令式 app.GetModels().then + app.GetSettings().then 改用 query 缓存，
  // 30s staleTime 内命中缓存不重复 fetch。
  useEffect(() => {
    if (!modelsData?.length) return;
    const key = settingsData?.selected_model_key || "";
    setModelKey(modelsData.find((m) => m.Key === key) ? key : modelsData[0].Key);
  }, [modelsData, settingsData]);

  const reset = useCallback(() => {
    setOpen(false);
    setError("");
    setskipped_count(0);
    setskipped_chapters([]);
    setFilePath("");
    setProgress({
      ...INITIAL_IMPORT_PROGRESS,
      message: t("novel.importPreparing2"),
    });
  }, [t]);

  const startImport = useCallback(
    async (fp?: string) => {
      setError("");
      setProgress({
        ...INITIAL_IMPORT_PROGRESS,
        stage: fp ? "parse" : "select_file",
        message: fp ? t("novel.importParsing2") : t("novel.importSelectFile2"),
      });
      setOpen(true);

      let result: imp.ImportResult | null;
      try {
        result = fp
          ? await ImportNovel({ file_path: fp })
          : await PickAndImportNovel();
      } catch (err: unknown) {
        setProgress((prev) => ({
          ...prev,
          stage: "error",
          message: t("novel.importRollbackDone"),
          percent: 100,
        }));
        setError(toErrorMessage(err, t("novel.importFailedRetry")));
        return;
      }

      if (!result) {
        reset();
        return;
      }

      // 正则分割失败，提示用户使用 AI 分析
      // 优先使用后端回传的 file_path（PickAndImportNovel 路径下 fp 为 undefined）
      if (result.needs_llm) {
        setFilePath(result.file_path || fp || "");
        setProgress((prev) => ({
          ...prev,
          stage: "needs_llm",
          message: t("novel.importNeedsLLM"),
          percent: 0,
        }));
        return;
      }

      setskipped_count(result.skipped_count ?? 0);
      setskipped_chapters(
        (result.skipped_chapters ?? []) as { title: string; reason: string }[],
      );

      try {
        await onImported(result);
      } catch (err: unknown) {
        setError(toErrorMessage(err, t("novel.importFailedRetry")));
      }
    },
    [onImported, reset, t],
  );

  // 用户点"AI 分析"→ 调 ImportWithLLM，LLM 分析后直接导入
  const startLLMImport = useCallback(async () => {
    if (!filePath || !modelKey) return;
    const [providerName, modelID] = splitModelKey(modelKey);
    if (!providerName || !modelID) return;

    setError("");
    setProgress((prev) => ({
      ...prev,
      stage: "analyzing",
      message: t("novel.importAnalyzing"),
      percent: 30,
    }));

    try {
      const result = await ImportWithLLM({
        file_path: filePath,
        provider_name: providerName,
        model_id: modelID,
      });

      setskipped_count(result.skipped_count ?? 0);
      setskipped_chapters(
        (result.skipped_chapters ?? []) as { title: string; reason: string }[],
      );

      await onImported(result);
    } catch (err: unknown) {
      setProgress((prev) => ({
        ...prev,
        stage: "error",
        message: t("novel.importRollbackDone"),
        percent: 100,
      }));
      setError(toErrorMessage(err, t("novel.importFailedRetry")));
    }
  }, [filePath, modelKey, onImported, t]);

  // modelOptions 直接用 query data（modelsData），不再同步到本地 state。
  const modelOptions = modelsData.map((m) => ({
    value: m.Key,
    label: m.ModelName,
  }));

  return {
    startImport,
    startLLMImport,
    modelKey,
    setModelKey,
    modelOptions,
    dialogProps: {
      open,
      progress,
      error,
      skippedCount,
      skippedChapters,
      onClose: reset,
    },
  };
}

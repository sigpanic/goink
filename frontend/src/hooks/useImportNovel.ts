import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import type { imp, llm, config } from "@/lib/wailsjs/go/models";
import type { app } from "@/lib/wailsjs/go/models";
import { EventsOn } from "@/lib/wailsjs/runtime/runtime";
import { toErrorMessage } from "@/utils/error";
import { splitModelKey } from "@/utils/modelKey";

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
  app: {
    ImportNovel: (input: app.ImportNovelInput) => Promise<imp.ImportResult>;
    PickAndImportNovel: () => Promise<imp.ImportResult>;
    ImportWithLLM: (input: app.ImportWithLLMInput) => Promise<imp.ImportResult>;
    GetModels: () => Promise<llm.AvailableModel[]>;
    GetSettings: () => Promise<config.AppSettings>;
    [key: string]: unknown;
  };
  onImported: (result: imp.ImportResult) => Promise<void>;
}

export function useImportNovel({ app, onImported }: UseImportNovelOptions) {
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
  const [modelKey, setModelKey] = useState("");
  const [models, setModels] = useState<llm.AvailableModel[]>([]);

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

  // 加载模型列表
  useEffect(() => {
    let cancelled = false;
    app
      .GetModels()
      .then((list) => {
        if (cancelled) return;
        if (list?.length) {
          setModels(list);
          app.GetSettings().then((s) => {
            if (cancelled) return;
            let key = s?.selected_model_key || "";
            if (!list.find((m) => m.Key === key)) key = list[0].Key;
            setModelKey(key);
          });
        }
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [app]);

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
          ? await app.ImportNovel({ file_path: fp })
          : await app.PickAndImportNovel();
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
    [app, onImported, reset, t],
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
      const result = await app.ImportWithLLM({
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
  }, [app, filePath, modelKey, onImported, t]);

  const modelOptions = models.map((m) => ({
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

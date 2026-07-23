import { useEffect, useRef, useState } from "react";
import { Loader2, Save, Sparkle } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useApp } from "@/hooks/useApp";
import { usePatternProgress } from "@/hooks/usePatternProgress";
import Markdown from "@/components/Markdown";
import { splitFrontmatter } from "@/components/content/types";
import PatternProgressView from "./PatternProgressView";

interface Props {
  taskId: string;
  novelId: number;
  chapterIds: number[]; // 空数组表示全书
  modelKey: string; // "provider/model" 格式
  title: string; // 小说标题（进度页展示用）
  chapterCount: number; // 本次提取的章节数
  onExit: () => void; // 返回选择页
}

type Status = "running" | "done" | "failed";

interface ExtractResult {
  taskId: string;
  novelId: number;
  name: string;
  description: string;
  filePath: string;
  rawContent: string;
}

function errorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error) return error.message;
  if (typeof error === "string") return error;
  return fallback;
}

export default function PatternSessionView({
  taskId,
  novelId,
  chapterIds,
  modelKey,
  title,
  chapterCount,
  onExit,
}: Props) {
  const app = useApp();
  const { t } = useTranslation();
  const [status, setStatus] = useState<Status>("running");
  const [result, setResult] = useState<ExtractResult | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const { progress, events, reset } = usePatternProgress(taskId);
  const startedRef = useRef(false);

  const runExtract = async () => {
    setStatus("running");
    setError("");
    setResult(null);
    reset();

    const [providerName, modelID] = modelKey.split("/");
    if (!providerName || !modelID) {
      setError(t("extract.extractFailed"));
      setStatus("failed");
      return;
    }

    try {
      const res = await app.ExtractPattern({
        task_id: taskId,
        novel_id: novelId,
        provider_name: providerName,
        model_id: modelID,
        reasoning_effort: "",
        chapter_ids: chapterIds.length > 0 ? chapterIds : undefined,
      });
      setResult({
        taskId: res.task_id || taskId,
        novelId,
        name: res.name,
        description: res.description,
        filePath: res.file_path,
        rawContent: res.raw_content,
      });
      setStatus("done");
    } catch (e: unknown) {
      const msg = errorMessage(e, "");
      if (!msg.includes("canceled") && !msg.includes("取消")) {
        setError(msg || t("extract.extractFailed"));
        setStatus("failed");
      }
    }
  };

  useEffect(() => {
    if (startedRef.current) return;
    startedRef.current = true;
    runExtract();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleCancel = async () => {
    try {
      await app.CancelExtractPattern(novelId);
    } catch {
      // ignore cancel errors
    }
    onExit();
  };

  const handleSave = async () => {
    if (!result) return;
    setLoading(true);
    setError("");
    try {
      await app.SaveContent({
        novel_id: result.novelId,
        path: result.filePath,
        content: result.rawContent,
      });
      onExit();
    } catch (e: unknown) {
      setError(errorMessage(e, t("extract.saveFailed")));
    } finally {
      setLoading(false);
    }
  };

  const { meta, body } = result
    ? splitFrontmatter(result.rawContent)
    : { meta: {}, body: "" };

  return (
    <div className="flex-1 flex flex-col min-h-0 bg-background">
      <div className="flex items-center justify-between px-6 py-4 border-b shrink-0">
        {status === "running" && (
          <>
            <span className="text-sm text-muted-foreground flex items-center gap-2">
              <Sparkle className="w-4 h-4 text-primary" />
              {t("extract.progress.title")}
            </span>
            <div className="flex items-center gap-2">
              <button
                onClick={handleCancel}
                className="inline-flex items-center gap-1.5 h-8 px-4 rounded-lg text-sm font-medium bg-destructive text-destructive-foreground hover:bg-destructive/80 transition-colors shadow-sm"
              >
                <Loader2 className="w-3.5 h-3.5 animate-spin" />
                {t("extract.cancel")}
              </button>
            </div>
          </>
        )}

        {status === "done" && result && (
          <>
            <span className="text-sm text-muted-foreground flex items-center gap-2">
              <Sparkle className="w-4 h-4 text-primary" />
              {t("extract.generated")}「{result.name}」
            </span>
            <div className="flex items-center gap-2">
              <button
                onClick={onExit}
                className="h-8 px-3 rounded-lg text-sm border border-border hover:bg-muted transition-colors"
              >
                {t("extract.cancel")}
              </button>
              <button
                onClick={runExtract}
                className="h-8 px-3 rounded-lg text-sm border border-border hover:bg-muted transition-colors"
              >
                {t("extract.reExtract")}
              </button>
              <button
                onClick={handleSave}
                disabled={loading}
                className="inline-flex items-center gap-1.5 h-8 px-4 rounded-lg text-sm font-medium bg-action-save text-action-save-foreground hover:bg-action-save/80 disabled:opacity-50 transition-colors"
              >
                <Save className="w-3.5 h-3.5" />
                {loading ? t("extract.saving") : t("extract.saveToUserSkill")}
              </button>
            </div>
          </>
        )}

        {status === "failed" && (
          <>
            <span className="text-sm text-muted-foreground flex items-center gap-2">
              <Sparkle className="w-4 h-4 text-primary" />
              {t("extract.extractFailed")}
            </span>
            <div className="flex items-center gap-2">
              <button
                onClick={runExtract}
                className="h-8 px-3 rounded-lg text-sm border border-border hover:bg-muted transition-colors"
              >
                {t("extract.reExtract")}
              </button>
              <button
                onClick={onExit}
                className="h-8 px-3 rounded-lg text-sm border border-border hover:bg-muted transition-colors"
              >
                {t("extract.cancel")}
              </button>
            </div>
          </>
        )}
      </div>

      {error && (
        <div className="mx-6 mt-3 px-3 py-2 text-xs text-destructive bg-danger-bg border border-danger-border rounded-md shrink-0">
          {error}
        </div>
      )}

      {status === "done" && result ? (
        <div className="flex-1 min-h-0 overflow-y-auto px-6 py-4 space-y-3">
          {Object.keys(meta).length > 0 && (
            <table className="border bg-muted/20 w-full text-sm rounded-lg overflow-hidden">
              <tbody>
                {meta.name && (
                  <tr className="border-b">
                    <td className="px-4 py-2 text-muted-foreground whitespace-nowrap w-16">
                      {t("extract.name")}
                    </td>
                    <td className="px-4 py-2 text-foreground font-semibold">
                      {meta.name}
                    </td>
                  </tr>
                )}
                {(meta.description || result.description) && (
                  <tr className="border-b">
                    <td className="px-4 py-2 text-muted-foreground whitespace-nowrap w-16">
                      {t("extract.summary")}
                    </td>
                    <td className="px-4 py-2 text-foreground">
                      {meta.description || result.description}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          )}
          <div className="rounded-lg border bg-muted/10 p-4">
            <Markdown content={body} />
          </div>
        </div>
      ) : (
        <div className="flex-1 min-h-0 overflow-y-auto p-6">
          <PatternProgressView
            progress={progress}
            events={events}
            novelTitle={title}
            chapterCount={chapterCount}
          />
        </div>
      )}
    </div>
  );
}

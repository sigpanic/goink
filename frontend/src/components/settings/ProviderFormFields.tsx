import { X, Loader2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { llm } from "@/lib/wailsjs/go/models";
import TemperatureInfo from "./TemperatureInfo";
import TestResultWithHint from "./TestResultWithHint";

interface Props {
  provider: llm.ProviderView;
  selectedKey: string;
  onUpdate: (key: string, patch: Partial<llm.ProviderView>) => void;
  onTest: (
    providerKey: string,
  ) => Promise<{ resolvedUrl?: string; error?: string }>;
  onRemoveCustomModel: (providerKey: string, modelId: string) => void;
  testResults: Record<
    string,
    { ok: boolean; msg?: string; warning?: string } | undefined
  >;
  testing: Record<string, boolean>;
  // 自定义模型列表容器的额外 className（Custom 用 "mb-2" 与下方 ModelDiscoveryPanel 隔开，Builtin 不用）
  customModelListClassName?: string;
}

/**
 * Provider 表单字段共享组件：ChatURL + APIKey + TestResult + Temperature + CustomModelList。
 * BuiltinProviderPane 和 CustomProviderPane 共用，避免重复代码。
 *
 * 字段顺序统一为 ChatURL → APIKey → TestResult → Temperature → CustomModelList
 * （Builtin 原来是 APIKey → ChatURL，已统一为 ChatURL → APIKey 更合理：先填端点再填 Key）。
 *
 * label 宽度统一为 w-16（Builtin 原来是 w-14，已统一为 w-16 更通用）。
 * Chat URL 提示 padding 统一为 pl-[5rem]（对应 w-16 + gap-3）。
 *
 * Builtin 独有的注册链接/指引/内置模型、Custom 独有的 name/新建表单/删除按钮，
 * 由各自 Pane 在调用 ProviderFormFields 前后插入，不在此组件内处理。
 */
export default function ProviderFormFields({
  provider,
  selectedKey,
  onUpdate,
  onTest,
  onRemoveCustomModel,
  testResults,
  testing,
  customModelListClassName = "",
}: Props) {
  const { t } = useTranslation();
  const isTesting = testing[selectedKey];
  const testResult = testResults[selectedKey];

  return (
    <>
      {/* Chat URL */}
      <div className="flex items-center gap-3">
        <label className="text-xs text-muted-foreground w-16 shrink-0">
          {t("settings.chatUrl")}
        </label>
        <input
          value={provider.chat_url}
          onChange={(e) => onUpdate(selectedKey, { chat_url: e.target.value })}
          className="flex-1 h-8 rounded-md border bg-background px-2.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
        />
      </div>
      <p className="text-xs text-muted-foreground pl-[5rem]">
        {t("settings.urlAutoDetectHint")}
      </p>

      {/* API Key + 测试按钮 */}
      <div className="flex items-center gap-2">
        <label className="text-xs text-muted-foreground w-16 shrink-0">
          {t("settings.apiKey")}
        </label>
        <input
          type="password"
          value={provider.api_key}
          onChange={(e) => onUpdate(selectedKey, { api_key: e.target.value })}
          placeholder={t("settings.enterApiKey")}
          className="flex-1 h-8 rounded-md border bg-background px-2.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
        />
        <button
          onClick={() => onTest(selectedKey)}
          disabled={!provider.api_key || isTesting}
          className="h-8 px-2.5 rounded-md border text-xs shrink-0 hover:bg-muted/50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          {isTesting ? (
            <Loader2 className="w-3.5 h-3.5 animate-spin" />
          ) : (
            t("settings.test")
          )}
        </button>
      </div>

      {/* 测试结果 */}
      <TestResultWithHint testResult={testResult} />

      {/* Temperature 滑块 */}
      <div className="flex items-center gap-3">
        <label className="text-xs text-muted-foreground w-16 shrink-0 flex items-center gap-1">
          {t("settings.creativity")}
          <TemperatureInfo />
        </label>
        <input
          type="range"
          min="0"
          max="2"
          step="0.1"
          value={provider.temperature}
          onChange={(e) =>
            onUpdate(selectedKey, { temperature: parseFloat(e.target.value) })
          }
          className="flex-1 h-8"
        />
        <span className="text-xs text-muted-foreground w-8 text-right">
          {(provider.temperature ?? 0.7).toFixed(1)}
        </span>
      </div>

      {/* 自定义模型列表 */}
      {provider.custom_models && provider.custom_models.length > 0 && (
        <div>
          <span className="text-xs text-muted-foreground mb-2 block">
            {t("settings.customModels")}
          </span>
          <div
            className={`rounded-md border divide-y ${customModelListClassName}`}
          >
            {provider.custom_models.map((m) => (
              <div
                key={m.id}
                className="flex items-center justify-between px-3 py-2"
              >
                <div>
                  <span className="text-sm">{m.name || m.id}</span>
                  {(m.context_window > 0 || m.max_output_tokens > 0) && (
                    <span className="text-xs text-muted-foreground ml-2">
                      {m.context_window > 0 &&
                        (m.context_window >= 1_000_000
                          ? (m.context_window / 1_000_000).toFixed(0) + "M"
                          : (m.context_window / 1_000).toFixed(0) + "K")}
                      {m.max_output_tokens > 0 && (
                        <>
                          {" "}
                          · {(m.max_output_tokens / 1_000).toFixed(0)}K{" "}
                          {t("settings.output")}
                        </>
                      )}
                      {m.supports_thinking ? (
                        <> · {t("settings.thinking")}</>
                      ) : null}
                      {m.reasoning_levels?.length ? (
                        <>
                          {" "}
                          · {t("settings.level")}:{" "}
                          {m.reasoning_levels.join(",")}
                        </>
                      ) : null}
                      {m.supports_vision ? (
                        <> · {t("settings.vision")}</>
                      ) : null}
                    </span>
                  )}
                </div>
                <button
                  onClick={() => onRemoveCustomModel(selectedKey, m.id)}
                  className="text-muted-foreground hover:text-destructive transition-colors"
                >
                  <X className="w-3.5 h-3.5" />
                </button>
              </div>
            ))}
          </div>
        </div>
      )}
    </>
  );
}

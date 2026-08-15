import { useState, useRef, useEffect } from "react";
import {
  X,
  CheckCircle2,
  Loader2,
  Globe,
  ExternalLink,
  ChevronDown,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { BrowserOpenURL } from "@/lib/wailsjs/runtime/runtime";
import type { llm } from "@/lib/wailsjs/go/models";
import TemperatureInfo from "./TemperatureInfo";
import ModelDiscoveryPanel from "./ModelDiscoveryPanel";
import ProviderIcon from "./ProviderIcon";
import TestResultWithHint from "./TestResultWithHint";

interface Props {
  providers: llm.ProviderView[];
  onUpdate: (key: string, patch: Partial<llm.ProviderView>) => void;
  onAddCustomModel: (providerKey: string, model: llm.ModelInfo) => void;
  onRemoveCustomModel: (providerKey: string, modelId: string) => void;
  onTest: (
    providerKey: string,
  ) => Promise<{ resolvedUrl?: string; error?: string }>;
  testResults: Record<string, { ok: boolean; msg?: string } | undefined>;
  testing: Record<string, boolean>;
}

export default function BuiltinProviderPane({
  providers,
  onUpdate,
  onAddCustomModel,
  onRemoveCustomModel,
  onTest,
  testResults,
  testing,
}: Props) {
  const { t } = useTranslation();
  const [selectedKey, setSelectedKey] = useState(providers[0]?.key || "");
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [helpOpen, setHelpOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);

  // 切换服务商时重置折叠和下拉状态
  useEffect(() => {
    setHelpOpen(false);
    setDropdownOpen(false);
  }, [selectedKey]);

  // providers 异步加载后同步 selectedKey：父组件 providers 初值为 []，
  // 首次挂载 selectedKey 被锁成 ""，providers 就绪后需自愈到首项 key
  useEffect(() => {
    if (providers.length > 0 && !providers.find((p) => p.key === selectedKey)) {
      setSelectedKey(providers[0].key);
    }
  }, [providers, selectedKey]);

  // 点击外部关闭下拉
  useEffect(() => {
    if (!dropdownOpen) return;
    const handle = (e: MouseEvent) => {
      if (
        dropdownRef.current &&
        !dropdownRef.current.contains(e.target as Node)
      ) {
        setDropdownOpen(false);
      }
    };
    document.addEventListener("mousedown", handle);
    return () => document.removeEventListener("mousedown", handle);
  }, [dropdownOpen]);

  const provider = providers.find((p) => p.key === selectedKey);
  if (!provider) {
    return (
      <div className="text-sm text-muted-foreground p-4">
        {t("settings.noBuiltinProviders")}
      </div>
    );
  }

  const hasKey = !!provider.api_key;
  const isTesting = testing[selectedKey];
  const testResult = testResults[selectedKey];

  const allExistingIds = new Set([
    ...(provider?.builtin_models || []).map((m) => m.id),
    ...(provider?.custom_models || []).map((m) => m.id),
  ]);

  return (
    <div className="flex flex-col gap-4">
      {/* 服务商选择 + 状态 */}
      <div className="flex items-center gap-3">
        <label className="text-xs text-muted-foreground w-14 shrink-0">
          {t("settings.provider")}
        </label>
        <div className="relative flex-1" ref={dropdownRef}>
          <button
            onClick={() => setDropdownOpen(!dropdownOpen)}
            className="flex items-center gap-2 w-full h-8 rounded-md border bg-background px-2.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
          >
            <ProviderIcon
              provider={selectedKey}
              className="w-4 h-4 shrink-0 text-muted-foreground"
            />
            <span className="flex-1 text-left">{provider.name}</span>
            <ChevronDown
              className={`w-3.5 h-3.5 shrink-0 text-muted-foreground transition-transform duration-200 ${dropdownOpen ? "rotate-180" : ""}`}
            />
          </button>
          {dropdownOpen && (
            <div className="absolute z-50 top-full left-0 right-0 mt-1 rounded-md border bg-popover text-popover-foreground shadow-md py-1 max-h-56 overflow-auto">
              {providers.map((p) => (
                <button
                  key={p.key}
                  onClick={() => {
                    setSelectedKey(p.key);
                    setDropdownOpen(false);
                  }}
                  className={`flex items-center gap-2 w-full px-2.5 py-1.5 text-sm hover:bg-muted/50 transition-colors ${p.key === selectedKey ? "bg-muted/30" : ""}`}
                >
                  <ProviderIcon
                    provider={p.key}
                    className="w-4 h-4 shrink-0 text-muted-foreground"
                  />
                  <span>{p.name}</span>
                </button>
              ))}
            </div>
          )}
        </div>
        <span
          className={`flex items-center gap-1 text-xs shrink-0 ${hasKey ? "text-success-foreground" : "text-muted-foreground"}`}
        >
          {hasKey ? (
            <>
              <CheckCircle2 className="w-3.5 h-3.5" />{" "}
              {t("settings.configured")}
            </>
          ) : (
            t("settings.notConfigured")
          )}
        </span>
      </div>

      {/* API Key + 测试 */}
      <div className="flex items-center gap-2">
        <label className="text-xs text-muted-foreground w-14 shrink-0">
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

      {/* Chat URL */}
      <div className="flex items-center gap-3">
        <label className="text-xs text-muted-foreground w-14 shrink-0">
          {t("settings.chatUrl")}
        </label>
        <input
          value={provider.chat_url}
          onChange={(e) => onUpdate(selectedKey, { chat_url: e.target.value })}
          className="flex-1 h-8 rounded-md border bg-background px-2.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
        />
      </div>
      <p className="text-xs text-muted-foreground pl-[4.5rem]">
        {t("settings.urlAutoDetectHint")}
      </p>

      {/* 测试结果 */}
      <TestResultWithHint testResult={testResult} />

      {/* 注册链接 */}
      {provider.platform_url && (
        <div className="flex items-center gap-3">
          <label className="text-xs text-muted-foreground w-14 shrink-0">
            {t("settings.register")}
          </label>
          <button
            onClick={() => BrowserOpenURL(provider.platform_url!)}
            className="flex items-center gap-1.5 h-8 px-2.5 rounded-md border text-xs hover:bg-muted/50 transition-colors max-w-full"
          >
            <Globe className="w-3.5 h-3.5 shrink-0" />
            <span className="truncate">{provider.platform_url}</span>
            <ExternalLink className="w-3 h-3 shrink-0 text-muted-foreground" />
          </button>
        </div>
      )}

      {/* 注册指引 */}
      {provider.help_text && (
        <div className="border rounded-md overflow-hidden">
          <button
            onClick={() => setHelpOpen(!helpOpen)}
            className="flex items-center gap-1.5 w-full px-3 py-2 text-xs text-muted-foreground hover:bg-muted/30 transition-colors"
          >
            <ChevronDown
              className={`w-3 h-3 transition-transform duration-200 ${helpOpen ? "rotate-180" : ""}`}
            />
            {t("settings.registerGuide")}
          </button>
          <div
            className={`grid transition-all duration-300 ease-out ${
              helpOpen
                ? "grid-rows-[1fr] opacity-100"
                : "grid-rows-[0fr] opacity-0"
            }`}
          >
            <div className="overflow-hidden">
              <div className="px-3 pb-2 text-xs text-muted-foreground leading-relaxed">
                {provider.help_text}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Temperature */}
      <div className="flex items-center gap-3">
        <label className="text-xs text-muted-foreground w-14 shrink-0 flex items-center gap-1">
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

      {/* 内置模型 */}
      {provider.builtin_models && provider.builtin_models.length > 0 && (
        <div>
          <div className="text-xs text-muted-foreground mb-2">
            {t("settings.builtinModels")}
          </div>
          <div className="rounded-md border divide-y">
            {provider.builtin_models.map((m) => (
              <div
                key={m.id}
                className="flex items-center justify-between px-3 py-2 bg-muted/30"
              >
                <span className="text-sm">{m.name}</span>
                <span className="text-xs text-muted-foreground">
                  {m.context_window >= 1_000_000
                    ? (m.context_window / 1_000_000).toFixed(0) + "M"
                    : (m.context_window / 1_000).toFixed(0) + "K"}{" "}
                  {t("settings.context")}
                  {m.max_output_tokens > 0 && (
                    <>
                      {" "}
                      · {(m.max_output_tokens / 1_000).toFixed(0)}K{" "}
                      {t("settings.output")}
                    </>
                  )}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* 自定义模型 */}
      {provider.custom_models && provider.custom_models.length > 0 && (
        <div>
          <span className="text-xs text-muted-foreground mb-2 block">
            {t("settings.customModels")}
          </span>
          <div className="rounded-md border divide-y">
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

      <ModelDiscoveryPanel
        key={selectedKey}
        chatUrl={provider.chat_url}
        apiKey={provider.api_key}
        existingIds={allExistingIds}
        onAddModel={(m) => onAddCustomModel(selectedKey, m)}
      />
    </div>
  );
}

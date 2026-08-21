import { useState, useEffect } from "react";
import { Globe, ExternalLink, ChevronDown } from "lucide-react";
import { useTranslation } from "react-i18next";
import { BrowserOpenURL } from "@/lib/wailsjs/runtime/runtime";
import type { llm } from "@/lib/wailsjs/go/models";
import ModelDiscoveryPanel from "./ModelDiscoveryPanel";
import ProviderIcon from "./ProviderIcon";
import ProviderFormFields from "./ProviderFormFields";
import ProviderDropdown from "./ProviderDropdown";
import ProviderStatusBadge from "./ProviderStatusBadge";

interface Props {
  providers: llm.ProviderView[];
  onUpdate: (key: string, patch: Partial<llm.ProviderView>) => void;
  onAddCustomModel: (providerKey: string, model: llm.ModelInfo) => void;
  onRemoveCustomModel: (providerKey: string, modelId: string) => void;
  onTest: (
    providerKey: string,
  ) => Promise<{ resolvedUrl?: string; error?: string }>;
  testResults: Record<string, { ok: boolean; msg?: string; warning?: string } | undefined>;
  testing: Record<string, boolean>;
  // 父组件外部指令：focusNonce 自增时 setSelectedKey(focusKey)，
  // 用于保存测试失败时把视图切到失败 provider 方便用户定位
  focusKey?: string;
  focusNonce?: number;
}

export default function BuiltinProviderPane({
  providers,
  onUpdate,
  onAddCustomModel,
  onRemoveCustomModel,
  onTest,
  testResults,
  testing,
  focusKey,
  focusNonce,
}: Props) {
  const { t } = useTranslation();
  const [selectedKey, setSelectedKey] = useState(providers[0]?.key || "");
  const [helpOpen, setHelpOpen] = useState(false);

  // 切换服务商时重置折叠状态（dropdown 已由 ProviderDropdown 内部管理）
  useEffect(() => {
    setHelpOpen(false);
  }, [selectedKey]);

  // providers 异步加载后同步 selectedKey：父组件 providers 初值为 []，
  // 首次挂载 selectedKey 被锁成 ""，providers 就绪后需自愈到首项 key
  useEffect(() => {
    if (providers.length > 0 && !providers.find((p) => p.key === selectedKey)) {
      setSelectedKey(providers[0].key);
    }
  }, [providers, selectedKey]);

  // 父组件外部指令：focusNonce 自增时强制选中 focusKey
  // （focusNonce 自增保证即便 focusKey 未变也能触发，比如连续失败同一 provider）
  useEffect(() => {
    if (focusKey && providers.find((p) => p.key === focusKey)) {
      setSelectedKey(focusKey);
    }
    // 故意只依赖 focusNonce，不依赖 focusKey/providers
    // （providers 变化时上面那个 effect 已处理，这里只响应外部指令）
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focusNonce]);

  const provider = providers.find((p) => p.key === selectedKey);
  if (!provider) {
    return (
      <div className="text-sm text-muted-foreground p-4">
        {t("settings.noBuiltinProviders")}
      </div>
    );
  }

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
        <ProviderDropdown
          providers={providers}
          selectedKey={selectedKey}
          onSelect={setSelectedKey}
          testResults={testResults}
          renderIcon={(key) => (
            <ProviderIcon
              provider={key}
              className="w-4 h-4 shrink-0 text-muted-foreground"
            />
          )}
        />
        <ProviderStatusBadge
          hasKey={!!provider.api_key}
          testResult={testResults[selectedKey]}
        />
      </div>

      {/* 共享表单字段：ChatURL → APIKey → TestResult → Temperature → CustomModelList */}
      <ProviderFormFields
        provider={provider}
        selectedKey={selectedKey}
        onUpdate={onUpdate}
        onTest={onTest}
        onRemoveCustomModel={onRemoveCustomModel}
        testResults={testResults}
        testing={testing}
      />

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

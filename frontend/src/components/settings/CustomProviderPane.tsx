import { useState } from "react";
import { Plus, X, Loader2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { llm } from "@/hooks/useApp";
import TemperatureInfo from "./TemperatureInfo";
import ModelDiscoveryPanel from "./ModelDiscoveryPanel";
import TestResultWithHint from "./TestResultWithHint";
import ConfirmDialog from "@/components/ui/ConfirmDialog";

interface Props {
  providers: llm.ProviderView[];
  onAdd: (provider: llm.ProviderView) => void;
  onUpdate: (key: string, patch: Partial<llm.ProviderView>) => void;
  onRemove: (key: string) => void;
  onAddCustomModel: (providerKey: string, model: llm.ModelInfo) => void;
  onRemoveCustomModel: (providerKey: string, modelId: string) => void;
  onTest: (providerKey: string) => Promise<{ resolvedUrl?: string; error?: string }>;
  testResults: Record<string, { ok: boolean; msg?: string } | undefined>;
  testing: Record<string, boolean>;
}

export default function CustomProviderPane({
  providers,
  onAdd,
  onUpdate,
  onRemove,
  onAddCustomModel,
  onRemoveCustomModel,
  onTest,
  testResults,
  testing,
}: Props) {
  const { t } = useTranslation();
  const [selectedKey, setSelectedKey] = useState(providers[0]?.key || "");
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
  const [showNewForm, setShowNewForm] = useState(false);

  // 删除自定义 provider：点按钮只记录目标弹确认框，确认后在 confirmDeleteProvider 里执行
  const confirmDeleteProvider = () => {
    if (!deleteTarget) return;
    onRemove(deleteTarget);
    setSelectedKey(providers.filter((p) => p.key !== deleteTarget)[0]?.key || "");
    setDeleteTarget(null);
  };
  const [newName, setNewName] = useState("");
  const [newChatURL, setNewChatURL] = useState("");
  const [newApiKey, setNewApiKey] = useState("");
  const provider = providers.find((p) => p.key === selectedKey);
  const isTesting = testing[selectedKey];
  const testResult = testResults[selectedKey];

  const handleAdd = () => {
    if (!newName.trim() || !newChatURL.trim()) return;
    onAdd({
      key: newName.trim().toLowerCase().replace(/\s+/g, "-"),
      name: newName.trim(),
      chat_url: newChatURL.trim(),
      api_key: newApiKey,
      source: "custom",
      builtin_models: [],
      custom_models: [],
      temperature: 0.7,
    } as unknown as llm.ProviderView);
    setNewName("");
    setNewChatURL("");
    setNewApiKey("");
    setShowNewForm(false);
    // 选中新加的 provider
    setSelectedKey(newName.trim().toLowerCase().replace(/\s+/g, "-"));
  };

  // 无自定义服务商且未展开新建表单
  if (providers.length === 0 && !showNewForm) {
    return (
      <div className="flex flex-col items-center gap-3 py-8">
        <p className="text-sm text-muted-foreground">
          {t("settings.noCustomProviders")}
        </p>
        <button
          onClick={() => setShowNewForm(true)}
          className="flex items-center gap-1 text-xs text-primary hover:underline"
        >
          <Plus className="w-3 h-3" /> {t("settings.addCustomProvider")}
        </button>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      {/* 服务商选择 + 添加 */}
      <div className="flex items-center gap-3">
        <label className="text-xs text-muted-foreground w-14 shrink-0">
          {t("settings.provider")}
        </label>
        <select
          value={selectedKey}
          onChange={(e) => setSelectedKey(e.target.value)}
          className="flex-1 h-8 rounded-md border bg-background px-2.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
        >
          {providers.map((p) => (
            <option key={p.key} value={p.key}>
              {p.name}
            </option>
          ))}
        </select>
        <button
          onClick={() => setShowNewForm(!showNewForm)}
          className="text-xs text-primary flex items-center gap-0.5 hover:underline shrink-0"
        >
          <Plus className="w-3 h-3" /> {t("settings.add")}
        </button>
      </div>

      {/* 新建表单 */}
      {showNewForm && (
        <div className="border rounded-md p-3 space-y-3">
          <div className="text-xs font-medium">
            {t("settings.newCustomProvider")}
          </div>

          <div className="flex items-center gap-3">
            <label className="text-xs text-muted-foreground w-16 shrink-0">
              {t("common.name")}
            </label>
            <input
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              placeholder={t("settings.providerName")}
              className="flex-1 h-8 rounded-md border bg-background px-2.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
            />
          </div>

          <div className="flex items-center gap-3">
            <label className="text-xs text-muted-foreground w-16 shrink-0">
              {t("settings.chatUrl")}
            </label>
            <input
              value={newChatURL}
              onChange={(e) => setNewChatURL(e.target.value)}
              placeholder="https://api.example.com/v1/chat/completions"
              className="flex-1 h-8 rounded-md border bg-background px-2.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
            />
          </div>

          <div className="flex items-center gap-3">
            <label className="text-xs text-muted-foreground w-16 shrink-0">
              {t("settings.apiKey")}
            </label>
            <input
              type="password"
              value={newApiKey}
              onChange={(e) => setNewApiKey(e.target.value)}
              placeholder={t("settings.enterApiKey")}
              className="flex-1 h-8 rounded-md border bg-background px-2.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
            />
          </div>

          <div className="flex items-center gap-2 pt-1">
            <div className="flex-1" />
            <button
              onClick={() => setShowNewForm(false)}
              className="h-8 px-3 rounded-md border text-xs text-muted-foreground"
            >
              {t("common.cancel")}
            </button>
            <button
              onClick={handleAdd}
              className="h-8 px-3 rounded-md bg-primary text-primary-foreground text-xs"
            >
              {t("settings.add")}
            </button>
          </div>
        </div>
      )}

      {/* 选中已有服务商时的编辑区 */}
      {provider && !showNewForm && (
        <>
          <div className="flex items-center gap-3">
            <label className="text-xs text-muted-foreground w-16 shrink-0">
              {t("common.name")}
            </label>
            <input
              value={provider.name}
              disabled
              className="flex-1 h-8 rounded-md border bg-muted/50 px-2.5 text-sm text-muted-foreground"
            />
          </div>

          <div className="flex items-center gap-3">
            <label className="text-xs text-muted-foreground w-16 shrink-0">
              {t("settings.chatUrl")}
            </label>
            <input
              value={provider.chat_url}
              onChange={(e) =>
                onUpdate(selectedKey, { chat_url: e.target.value })
              }
              className="flex-1 h-8 rounded-md border bg-background px-2.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
            />
          </div>
          <p className="text-xs text-muted-foreground pl-[5rem]">
            {t("settings.urlAutoDetectHint")}
          </p>

          <div className="flex items-center gap-2">
            <label className="text-xs text-muted-foreground w-16 shrink-0">
              {t("settings.apiKey")}
            </label>
            <input
              type="password"
              value={provider.api_key}
              onChange={(e) =>
                onUpdate(selectedKey, { api_key: e.target.value })
              }
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
                onUpdate(selectedKey, {
                  temperature: parseFloat(e.target.value),
                })
              }
              className="flex-1 h-8"
            />
            <span className="text-xs text-muted-foreground w-8 text-right">
              {(provider.temperature ?? 0.7).toFixed(1)}
            </span>
          </div>

          {/* 模型列表 */}
          {provider.custom_models && provider.custom_models.length > 0 && (
            <div>
              <span className="text-xs text-muted-foreground mb-2 block">
                {t("settings.customModels")}
              </span>
              <div className="rounded-md border divide-y mb-2">
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
            existingIds={
              new Set((provider?.custom_models || []).map((m) => m.id))
            }
            onAddModel={(m) => onAddCustomModel(selectedKey, m)}
          />

          {/* 删除 */}
          <div className="flex pt-1">
            <button
              onClick={() => setDeleteTarget(selectedKey)}
              className="h-8 px-3 rounded-md border border-danger-border text-destructive text-xs hover:bg-danger-bg transition-colors"
            >
              {t("settings.deleteProvider")}
            </button>
          </div>
        </>
      )}
      <ConfirmDialog
        open={deleteTarget !== null}
        title={t("settings.confirmDeleteProvider")}
        message={
          deleteTarget
            ? `"${providers.find((p) => p.key === deleteTarget)?.name || deleteTarget}"？${t("common.irreversible")}`
            : ""
        }
        danger
        confirmText={t("common.delete")}
        onClose={() => setDeleteTarget(null)}
        onConfirm={confirmDeleteProvider}
      />
    </div>
  );
}

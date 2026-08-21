import { useState, useEffect } from "react";
import { Plus } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { llm } from "@/lib/wailsjs/go/models";
import ModelDiscoveryPanel from "./ModelDiscoveryPanel";
import ProviderFormFields from "./ProviderFormFields";
import ProviderDropdown from "./ProviderDropdown";
import ProviderStatusBadge from "./ProviderStatusBadge";
import ConfirmDialog from "@/components/ui/ConfirmDialog";

interface Props {
  providers: llm.ProviderView[];
  onAdd: (provider: llm.ProviderView) => void;
  onUpdate: (key: string, patch: Partial<llm.ProviderView>) => void;
  onRemove: (key: string) => void;
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
  focusKey,
  focusNonce,
}: Props) {
  const { t } = useTranslation();
  const [selectedKey, setSelectedKey] = useState(providers[0]?.key || "");
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
  const [showNewForm, setShowNewForm] = useState(false);

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

  // 删除自定义 provider：点按钮只记录目标弹确认框，确认后在 confirmDeleteProvider 里执行
  const confirmDeleteProvider = () => {
    if (!deleteTarget) return;
    onRemove(deleteTarget);
    setSelectedKey(
      providers.filter((p) => p.key !== deleteTarget)[0]?.key || "",
    );
    setDeleteTarget(null);
  };
  const [newName, setNewName] = useState("");
  const [newChatURL, setNewChatURL] = useState("");
  const [newApiKey, setNewApiKey] = useState("");
  const provider = providers.find((p) => p.key === selectedKey);

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
      {/* 服务商选择 + 状态 + 添加 */}
      <div className="flex items-center gap-3">
        <label className="text-xs text-muted-foreground w-14 shrink-0">
          {t("settings.provider")}
        </label>
        <ProviderDropdown
          providers={providers}
          selectedKey={selectedKey}
          onSelect={setSelectedKey}
          testResults={testResults}
          renderIcon={(_key, name) => {
            // Custom 没有内置图标，用首字母圆形 fallback
            // 取 name 首字母（不足用 ?），大写后放在 bg-primary/15 圆形里
            const initial = (name || "?").charAt(0).toUpperCase();
            return (
              <div className="w-4 h-4 rounded-full bg-primary/15 text-primary text-[10px] flex items-center justify-center font-medium shrink-0">
                {initial}
              </div>
            );
          }}
        />
        <ProviderStatusBadge
          hasKey={!!provider?.api_key}
          testResult={provider ? testResults[provider.key] : undefined}
        />
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

          {/* 共享表单字段：ChatURL → APIKey → TestResult → Temperature → CustomModelList */}
          <ProviderFormFields
            provider={provider}
            selectedKey={selectedKey}
            onUpdate={onUpdate}
            onTest={onTest}
            onRemoveCustomModel={onRemoveCustomModel}
            testResults={testResults}
            testing={testing}
            customModelListClassName="mb-2"
          />

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

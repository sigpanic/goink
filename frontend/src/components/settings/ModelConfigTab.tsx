import { useState, useEffect, useCallback, useRef } from "react";
import { Loader2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useApp } from "@/hooks/useApp";
import type { llm } from "@/hooks/useApp";
import { toastSuccess } from "@/lib/utils";
import BuiltinProviderPane from "./BuiltinProviderPane";
import CustomProviderPane from "./CustomProviderPane";

type SubNav = "builtin" | "custom";

interface Props {
  onSaved?: () => void;
}

export default function ModelConfigTab({ onSaved }: Props) {
  const { t } = useTranslation();
  const app = useApp();
  const [providers, setProviders] = useState<llm.ProviderView[]>([]);
  const [subNav, setSubNav] = useState<SubNav>("builtin");
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [saveMsg, setSaveMsg] = useState("");

  // 测试状态：{ providerKey: { ok, msg, apiKey } }
  const [testResults, setTestResults] = useState<
    Record<
      string,
      { ok: boolean; msg?: string; keySnapshot: string } | undefined
    >
  >({});
  const [testing, setTesting] = useState<Record<string, boolean>>({});
  // 保存过后的配置哈希，用于判断 key 是否被修改
  const savedKeysRef = useRef<Record<string, string>>({});

  useEffect(() => {
    app
      .GetLLMConfig()
      .then((config) => {
        if (config?.providers) {
          setProviders(config.providers);
          const keys: Record<string, string> = {};
          for (const p of config.providers) {
            if (p.api_key) keys[p.key] = p.api_key;
          }
          savedKeysRef.current = keys;
        }
      })
      .catch(() => {})
      .finally(() => setIsLoading(false));
  }, [app]);

  const builtinProviders = providers.filter((p) => p.source === "builtin");
  const customProviders = providers.filter((p) => p.source === "custom");

  const handleUpdateProvider = useCallback(
    (key: string, patch: Partial<llm.ProviderView>) => {
      setProviders((prev) =>
        prev.map((p) =>
          p.key === key
            ? ({ ...p, ...patch } as unknown as llm.ProviderView)
            : p,
        ),
      );
      // key 变了就清除旧测试结果
      if ("api_key" in patch) {
        setTestResults((prev) => {
          if (prev[key]?.keySnapshot === patch.api_key) return prev;
          const next = { ...prev };
          delete next[key];
          return next;
        });
      }
    },
    [],
  );

  const handleAddCustomProvider = useCallback((provider: llm.ProviderView) => {
    setProviders((prev) => [...prev, provider]);
  }, []);

  const handleRemoveCustomProvider = useCallback((key: string) => {
    setProviders((prev) => prev.filter((p) => p.key !== key));
  }, []);

  const handleAddCustomModel = useCallback(
    (providerKey: string, model: llm.ModelInfo) => {
      setProviders((prev) =>
        prev.map((p) => {
          if (p.key !== providerKey) return p;
          const models = [...(p.custom_models || []), model];
          return { ...p, custom_models: models } as unknown as llm.ProviderView;
        }),
      );
    },
    [],
  );

  const handleRemoveCustomModel = useCallback(
    (providerKey: string, modelId: string) => {
      setProviders((prev) =>
        prev.map((p) => {
          if (p.key !== providerKey) return p;
          const models = (p.custom_models || []).filter(
            (m) => m.id !== modelId,
          );
          return { ...p, custom_models: models } as unknown as llm.ProviderView;
        }),
      );
    },
    [],
  );

  // 测试连通性，返回 { resolvedUrl?, error? }。
  // 后端多层 fallback 真测，返回验证通过的实际 URL（可能和入参不同）。
  // 成功时把 resolvedUrl 回写到 provider.chat_url，确保保存的 URL 和测试时一致。
  const handleTest = useCallback(
    async (
      providerKey: string,
    ): Promise<{ resolvedUrl?: string; error?: string }> => {
      const provider = providers.find((p) => p.key === providerKey);
      if (!provider || !provider.api_key) {
        const msg = t("settings.apiKeyNotConfigured");
        setTestResults((prev) => ({
          ...prev,
          [providerKey]: { ok: false, msg, keySnapshot: "" },
        }));
        return { error: msg };
      }

      const models = provider.builtin_models?.length
        ? provider.builtin_models
        : provider.custom_models;
      const modelId = models?.[0]?.id;
      if (!modelId) {
        const msg = t("settings.pleaseAddModel");
        setTestResults((prev) => ({
          ...prev,
          [providerKey]: { ok: false, msg, keySnapshot: provider.api_key },
        }));
        return { error: msg };
      }

      setTesting((prev) => ({ ...prev, [providerKey]: true }));
      try {
        // 后端 expandChatURLCandidates 会补 https:// 和多层 fallback，
        // 前端直接传原值，不再自己 norl。
        const resolvedUrl = await app.TestConnection({
          provider_name: providerKey,
          chat_url: provider.chat_url || "",
          api_key: provider.api_key,
          model_id: modelId,
        });
        // 回写探测到的正确 URL（多层 fallback 可能和原值不同）
        if (resolvedUrl && resolvedUrl !== provider.chat_url) {
          setProviders((prev) =>
            prev.map((p) =>
              p.key === providerKey
                ? ({ ...p, chat_url: resolvedUrl } as unknown as llm.ProviderView)
                : p,
            ),
          );
          // 提示用户 URL 已被自动补全（避免对输入框变化感到意外）
          toastSuccess(t("settings.urlAutoCompleted", { url: resolvedUrl }));
        }
        setTestResults((prev) => ({
          ...prev,
          [providerKey]: { ok: true, keySnapshot: provider.api_key },
        }));
        return { resolvedUrl };
      } catch (err: any) {
        const msg = String(err).replace(/^app: test connection: /, "");
        setTestResults((prev) => ({
          ...prev,
          [providerKey]: { ok: false, msg, keySnapshot: provider.api_key },
        }));
        return { error: msg };
      } finally {
        setTesting((prev) => ({ ...prev, [providerKey]: false }));
      }
    },
    [providers, app, t],
  );

  const handleSave = useCallback(async () => {
    // 收集有 key 的 provider
    const withKey = providers.filter((p) => p.api_key);
    if (withKey.length === 0) {
      // 错误消息不清空（保留到下次成功保存或新错误覆盖），让用户看清错误内容
      setSaveMsg(t("settings.pleaseConfigureApiKey"));
      return;
    }

    // 找出需要测试的：从未测试过，或 key 跟上次测试/保存时不一致
    const needTest = withKey.filter((p) => {
      const tr = testResults[p.key];
      if (!tr || !tr.ok) return true; // 从未测试或上次失败
      if (tr.keySnapshot !== p.api_key) return true; // key 变了
      return false;
    });

    // 测试时探测到的正确 URL，用于覆盖保存数据（闭包 providers 是旧的，用 resolvedUrls 兜底）
    const resolvedUrls: Record<string, string> = {};
    if (needTest.length > 0) {
      setSaveMsg(t("settings.testingConnection"));
      for (const p of needTest) {
        const result = await handleTest(p.key);
        if (result.error) {
          // 错误消息不清空（保留到下次成功保存或新错误覆盖），让用户看清错误内容
          setSaveMsg(
            `${p.name} ${t("settings.connectionTestFailed")}: ${result.error}`,
          );
          return;
        }
        if (result.resolvedUrl) {
          resolvedUrls[p.key] = result.resolvedUrl;
        }
      }
    }

    setIsSaving(true);
    setSaveMsg("");
    try {
      // 用 resolvedUrls 覆盖 chat_url，构造保存数据
      // （handleTest 内部已 setProviders 回写 UI，这里同步保存数据）
      const providersToSave = providers.map((p) => {
        const resolved = resolvedUrls[p.key];
        return resolved && resolved !== p.chat_url
          ? ({ ...p, chat_url: resolved } as unknown as llm.ProviderView)
          : p;
      });
      await app.SaveLLMConfig({
        providers: providersToSave,
      } as unknown as llm.LLMConfigView);
      const keys: Record<string, string> = {};
      for (const p of providers) {
        if (p.api_key) keys[p.key] = p.api_key;
      }
      savedKeysRef.current = keys;
      setSaveMsg(t("settings.configSaved"));
      onSaved?.();
      setTimeout(() => setSaveMsg(""), 2000);
    } catch (err) {
      setSaveMsg(`${t("settings.saveFailed")}: ${String(err)}`);
    } finally {
      setIsSaving(false);
    }
  }, [providers, app, testResults, handleTest, t, onSaved]);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader2 className="w-5 h-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      {/* 子导航 */}
      <div className="flex gap-6 px-1 mb-4">
        <button
          onClick={() => setSubNav("builtin")}
          className={`text-sm pb-1 transition-colors ${
            subNav === "builtin"
              ? "text-foreground border-b-2 border-primary font-medium"
              : "text-muted-foreground hover:text-foreground"
          }`}
        >
          {t("settings.builtinProviders")}
        </button>
        <button
          onClick={() => setSubNav("custom")}
          className={`text-sm pb-1 transition-colors ${
            subNav === "custom"
              ? "text-foreground border-b-2 border-primary font-medium"
              : "text-muted-foreground hover:text-foreground"
          }`}
        >
          {t("settings.customProviders")}
        </button>
      </div>

      {/* 内容区 */}
      <div className="flex-1 overflow-y-auto">
        {subNav === "builtin" ? (
          <BuiltinProviderPane
            providers={builtinProviders}
            onUpdate={handleUpdateProvider}
            onAddCustomModel={handleAddCustomModel}
            onRemoveCustomModel={handleRemoveCustomModel}
            onTest={handleTest}
            testResults={testResults}
            testing={testing}
          />
        ) : (
          <CustomProviderPane
            providers={customProviders}
            onAdd={handleAddCustomProvider}
            onUpdate={handleUpdateProvider}
            onRemove={handleRemoveCustomProvider}
            onAddCustomModel={handleAddCustomModel}
            onRemoveCustomModel={handleRemoveCustomModel}
            onTest={handleTest}
            testResults={testResults}
            testing={testing}
          />
        )}
      </div>

      {/* 底部保存栏 */}
      <div className="flex items-center justify-end gap-3 pt-4 border-t mt-4">
        {saveMsg && (
          <span
            className={`text-xs ${saveMsg.includes("失败") || saveMsg.includes("测试") ? "text-red-500" : "text-success-foreground"}`}
          >
            {saveMsg}
          </span>
        )}
        <button
          onClick={handleSave}
          disabled={isSaving}
          className="h-8 px-4 rounded-md bg-primary text-primary-foreground text-sm disabled:opacity-50"
        >
          {isSaving ? t("common.saving") : t("settings.saveConfig")}
        </button>
      </div>
    </div>
  );
}

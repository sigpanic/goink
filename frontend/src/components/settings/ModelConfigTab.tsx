import { useState, useEffect, useCallback, useRef } from "react";
import { Loader2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { llm } from "@/lib/wailsjs/go/models";
import { toastSuccess } from "@/utils/toast";
import { useSaveLLMConfig } from "./useSaveLLMConfig";
import { useLLMConfig } from "./useLLMConfig";
import { useTestConnection } from "./useTestConnection";
import BuiltinProviderPane from "./BuiltinProviderPane";
import CustomProviderPane from "./CustomProviderPane";
import ConfirmDialog from "@/components/ui/ConfirmDialog";

type SubNav = "builtin" | "custom";

export default function ModelConfigTab() {
  const { t } = useTranslation();
  const saveMutation = useSaveLLMConfig();
  const llmConfigQuery = useLLMConfig();
  const testConnectionMutation = useTestConnection();
  const [providers, setProviders] = useState<llm.ProviderView[]>([]);
  const [subNav, setSubNav] = useState<SubNav>("builtin");
  const [isSaving, setIsSaving] = useState(false);
  const [saveMsg, setSaveMsg] = useState("");

  // 测试状态：{ providerKey: { ok, msg, warning, apiKey } }
  // warning 用于 429 限流等"视为通过但有提示"的场景，不影响 ok 判定
  const [testResults, setTestResults] = useState<
    Record<
      string,
      { ok: boolean; msg?: string; warning?: string; keySnapshot: string } | undefined
    >
  >({});
  const [testing, setTesting] = useState<Record<string, boolean>>({});
  // 用户已知晓失败的 provider key 集合。
  // 一旦保存时某 provider 测试失败，加入此集合，下次保存跳过重测
  // （除非用户改了该 provider 的 api_key 或 chat_url，handleUpdateProvider 会清）
  const [acknowledgedFailures, setAcknowledgedFailures] = useState<
    Set<string>
  >(new Set());
  // 保存过后的配置哈希，用于判断 key 是否被修改
  const savedKeysRef = useRef<Record<string, string>>({});

  // 父组件对子组件 selectedKey 的"外部指令"：focusKey 指定要选中的 provider key，
  // focusNonce 每次 focusOn 自增用于触发子组件 useEffect（即便 focusKey 未变也能强制选中）。
  // 子组件 selectedKey 主权仍在自己（用户手动切换、providers 异步加载同步等逻辑不变），
  // 父组件只在保存测试失败时通过 focusOn 切到失败 provider 方便用户定位。
  const [focusKey, setFocusKey] = useState("");
  const [focusNonce, setFocusNonce] = useState(0);
  const focusOn = useCallback((key: string) => {
    setFocusKey(key);
    setFocusNonce((n) => n + 1);
  }, []);

  // 失败确认框：保存时若某 provider 测试失败，弹此框询问是否保留。
  // pendingFailure 是要展示给用户的信息（providerKey/name/error）；
  // resolverRef 持有 Promise 的 resolve，用户点击后注入 true（保留）/ false（取消）。
  // handleSave 在 await confirmFailure 时会暂停，等用户选择后再继续后续 provider 测试或 return。
  const [pendingFailure, setPendingFailure] = useState<{
    providerKey: string;
    name: string;
    error: string;
  } | null>(null);
  const resolverRef = useRef<((ok: boolean) => void) | null>(null);
  const confirmFailure = useCallback(
    (providerKey: string, name: string, error: string): Promise<boolean> => {
      return new Promise((resolve) => {
        resolverRef.current = resolve;
        setPendingFailure({ providerKey, name, error });
      });
    },
    [],
  );
  const resolveFailure = useCallback((ok: boolean) => {
    if (resolverRef.current) {
      resolverRef.current(ok);
      resolverRef.current = null;
    }
    setPendingFailure(null);
  }, []);

  // query data ready 后一次性初始化 providers/savedKeysRef/testResults（providers 是本地编辑态，
  // 后续 handleUpdateProvider/handleTest/handleSave 都改本地 state，不回写 query cache）。
  // GET 错误由全局中间件接管（llm-config 前缀 → settings.llmConfigLoadFailed），不在此处理。
  //
  // testResults 初始化：把所有有 key 的 provider 默认标记为 ok=true（keySnapshot=api_key）。
  // 语义：后端保存的配置默认假设有效（上次保存时已通过测试），打开后未改 url/key 的不重测，
  // 改了 api_key/chat_url 会由 handleUpdateProvider 清除 testResults 触发重测。
  // 代价：若后端 URL 突然挂了或用户在外部手改了配置文件，会被默认通过；但用户主动点测试能发现。
  useEffect(() => {
    const config = llmConfigQuery.data;
    if (config?.providers) {
      setProviders(config.providers);
      const keys: Record<string, string> = {};
      const results: Record<
        string,
        { ok: boolean; msg?: string; warning?: string; keySnapshot: string } | undefined
      > = {};
      for (const p of config.providers) {
        if (p.api_key) {
          keys[p.key] = p.api_key;
          results[p.key] = { ok: true, keySnapshot: p.api_key };
        }
      }
      savedKeysRef.current = keys;
      setTestResults(results);
    }
  }, [llmConfigQuery.data]);

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
      // api_key 或 chat_url 改动都会使旧的 testResults 失效（dirty 校验：
      // 未测/key变/url变 都要在下次保存时重测，否则会保存未验证的新 URL）
      if ("api_key" in patch || "chat_url" in patch) {
        setTestResults((prev) => {
          // 优化：仅 api_key 改动且值未变（onChange 触发但实际内容相同），跳过
          if (
            "api_key" in patch &&
            !("chat_url" in patch) &&
            prev[key]?.keySnapshot === patch.api_key
          ) {
            return prev;
          }
          const next = { ...prev };
          delete next[key];
          return next;
        });
        // 改 api_key/chat_url 后，旧的"已知晓失败"状态也失效：下次保存应重测
        setAcknowledgedFailures((prev) => {
          if (!prev.has(key)) return prev;
          const next = new Set(prev);
          next.delete(key);
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

  // 测试连通性，返回 { resolvedUrl?, warning?, error? }。
  // 后端多层 fallback 真测，返回验证通过的实际 URL（可能和入参不同）。
  // 成功时把 resolvedUrl 回写到 provider.chat_url，确保保存的 URL 和测试时一致。
  // warning 非空表示"视为通过但有提示"（如 429 限流），不影响 ok 判定但展示给用户。
  const handleTest = useCallback(
    async (
      providerKey: string,
    ): Promise<{ resolvedUrl?: string; warning?: string; error?: string }> => {
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
        // 后端返回 { url, warning }：url 是验证通过的实际端点，warning 是非致命提示
        const result = await testConnectionMutation.mutateAsync({
          provider_name: providerKey,
          chat_url: provider.chat_url || "",
          api_key: provider.api_key,
          model_id: modelId,
        });
        const resolvedUrl = result.url;
        const warning = result.warning;
        // 回写探测到的正确 URL（多层 fallback 可能和原值不同）
        if (resolvedUrl && resolvedUrl !== provider.chat_url) {
          setProviders((prev) =>
            prev.map((p) =>
              p.key === providerKey
                ? ({
                    ...p,
                    chat_url: resolvedUrl,
                  } as unknown as llm.ProviderView)
                : p,
            ),
          );
          // 提示用户 URL 已被自动补全（避免对输入框变化感到意外）
          toastSuccess(t("settings.urlAutoCompleted", { url: resolvedUrl }));
        }
        setTestResults((prev) => ({
          ...prev,
          [providerKey]: {
            ok: true,
            warning,
            keySnapshot: provider.api_key,
          },
        }));
        // 测试通过：清掉旧的"已知晓失败"状态（如果有）
        setAcknowledgedFailures((prev) => {
          if (!prev.has(providerKey)) return prev;
          const next = new Set(prev);
          next.delete(providerKey);
          return next;
        });
        return { resolvedUrl, warning };
      } catch (err: any) {
        const msg = String(err).replace(/^app: test connection: /, "");
        setTestResults((prev) => ({
          ...prev,
          [providerKey]: { ok: false, msg, keySnapshot: provider.api_key },
        }));
        // 不在此自动加入 acknowledgedFailures：是否保留失败配置由 handleSave
        // 弹确认框后由用户决定（用户改 api_key/chat_url 后 handleUpdateProvider 会清，触发重测）
        return { error: msg };
      } finally {
        setTesting((prev) => ({ ...prev, [providerKey]: false }));
      }
    },
    [providers, testConnectionMutation.mutateAsync, t],
  );

  const handleSave = useCallback(async () => {
    // 收集有 key 的 provider
    const withKey = providers.filter((p) => p.api_key);
    if (withKey.length === 0) {
      // 错误消息不清空（保留到下次成功保存或新错误覆盖），让用户看清错误内容
      setSaveMsg(t("settings.pleaseConfigureApiKey"));
      return;
    }

    // 找出需要测试的：从未测过 / key 变了 / 上次失败但用户未知晓
    const needTest = withKey.filter((p) => {
      const tr = testResults[p.key];
      if (!tr) return true; // 从未测试
      if (!tr.ok) {
        // 上次失败：用户已知晓则跳过（避免每次保存都重测已知失败），
        // 用户改 api_key/chat_url 后 handleUpdateProvider 会清，触发重测
        return !acknowledgedFailures.has(p.key);
      }
      if (tr.keySnapshot !== p.api_key) return true; // key 变了
      return false;
    });

    // 测试时探测到的正确 URL，用于覆盖保存数据（闭包 providers 是旧的，用 resolvedUrls 兜底）
    const resolvedUrls: Record<string, string> = {};
    // 收集本次保存中被用户"保留"的失败 provider name，最后汇总提示
    const keptFailures: string[] = [];
    if (needTest.length > 0) {
      setSaveMsg(t("settings.testingConnection"));
      for (const p of needTest) {
        const result = await handleTest(p.key);
        if (result.error) {
          // 弹确认框：用户决定是否保留此失败配置
          // 选"保留"→ 加入 acknowledgedFailures，下次保存跳过该 provider，继续测下一个
          // 选"取消"→ 不保存，切到失败 provider 视图，让用户先修复
          const keep = await confirmFailure(p.key, p.name, result.error);
          if (!keep) {
            // 错误消息不清空（保留到下次成功保存或新错误覆盖），让用户看清错误内容
            // ✗ 前缀让用户一眼看出是失败消息（颜色已是红色，符号强化识别）
            setSaveMsg(
              `✗ ${p.name} ${t("settings.connectionTestFailed")}: ${result.error}`,
            );
            // 切换到失败 provider 所在的视图并选中该 provider，方便用户定位
            setSubNav(p.source === "builtin" ? "builtin" : "custom");
            focusOn(p.key);
            return;
          }
          // 用户选"保留"：标记为已知晓失败，下次保存自动跳过该 provider 测试
          // （除非用户改了 api_key/chat_url，handleUpdateProvider 会清，触发重测）
          setAcknowledgedFailures((prev) => {
            const next = new Set(prev);
            next.add(p.key);
            return next;
          });
          // 记录被保留的失败 provider name，最后汇总提示
          keptFailures.push(p.name);
          // 提示用户此 provider 已被保留为已知失败，不在保存数据中改 URL
          // （resolvedUrls 不写入，使用 providers 中原值）
          continue;
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
      await saveMutation.mutateAsync({
        providers: providersToSave,
      } as unknown as llm.LLMConfigView);
      const keys: Record<string, string> = {};
      for (const p of providers) {
        if (p.api_key) keys[p.key] = p.api_key;
      }
      savedKeysRef.current = keys;
      // 根据 keptFailures 构造成功消息：含保留信息时显示完整提示，
      // 无保留时仅显示"配置已保存"
      const successMsg = keptFailures.length > 0
        ? t("settings.configSavedWithKept", { names: keptFailures.join(t("common.listSeparator")) })
        : t("settings.configSaved");
      // ✓ 前缀让用户一眼看出是成功消息（颜色已是绿色，符号强化识别）
      // saveMsg 不主动清理，保留显示直到下次保存（成功/失败/测试中）结果覆盖
      setSaveMsg(`✓ ${successMsg}`);
      // 额外用 toast 通知保存成功（采用项目 toast 方式）
      toastSuccess(successMsg);
    } catch (err) {
      setSaveMsg(`✗ ${t("settings.saveFailed")}: ${String(err)}`);
    } finally {
      setIsSaving(false);
    }
  }, [providers, saveMutation.mutateAsync, testResults, acknowledgedFailures, handleTest, focusOn, confirmFailure, t]);

  if (llmConfigQuery.isLoading) {
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
            focusKey={focusKey}
            focusNonce={focusNonce}
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
            focusKey={focusKey}
            focusNonce={focusNonce}
          />
        )}
      </div>

      {/* 底部保存栏 */}
      <div className="flex items-end justify-between gap-3 pt-4 border-t mt-4">
        <span
          className={`flex-1 text-xs whitespace-pre-line max-w-[60ch] text-left ${saveMsg.includes("失败") || saveMsg.includes("测试") ? "text-destructive" : "text-success-foreground"}`}
        >
          {saveMsg}
        </span>
        <button
          onClick={handleSave}
          disabled={isSaving}
          className="h-8 px-4 rounded-md bg-primary text-primary-foreground text-sm disabled:opacity-50 shrink-0"
        >
          {isSaving ? t("common.saving") : t("settings.saveConfig")}
        </button>
      </div>

      {/* 保存测试失败时的确认框：用户决定是否保留失败配置 */}
      <ConfirmDialog
        open={pendingFailure !== null}
        title={t("settings.confirmKeepFailureTitle")}
        message={
          pendingFailure
            ? t("settings.confirmKeepFailureMessage", {
                name: pendingFailure.name,
                error: pendingFailure.error,
              })
            : ""
        }
        confirmText={t("settings.keepAnyway")}
        cancelText={t("common.cancel")}
        onConfirm={() => resolveFailure(true)}
        onClose={() => resolveFailure(false)}
      />
    </div>
  );
}

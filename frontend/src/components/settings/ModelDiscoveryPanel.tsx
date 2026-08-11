import { useState } from "react";
import { Plus, X, Search, Loader2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { llm } from "@/lib/wailsjs/go/models";
import { DiscoverModels } from "@/lib/wailsjs/go/app/App";
import { toErrorMessage } from "@/utils/error";
import ModelEditForm from "./ModelEditForm";
import TestResultWithHint from "./TestResultWithHint";

interface Props {
  chatUrl: string;
  apiKey: string;
  existingIds: Set<string>;
  onAddModel: (model: llm.ModelInfo) => void;
}

const DEFAULT_CTX = 200_000;
const DEFAULT_MAX = 64_000;

const emptyModel = (): llm.ModelInfo =>
  ({
    id: "",
    name: "",
    context_window: DEFAULT_CTX,
    max_output_tokens: DEFAULT_MAX,
    supports_thinking: true,
    supports_vision: false,
  }) as unknown as llm.ModelInfo;

export default function ModelDiscoveryPanel({
  chatUrl,
  apiKey,
  existingIds,
  onAddModel,
}: Props) {
  const { t } = useTranslation();
  // 手动添加
  const [showAddForm, setShowAddForm] = useState(false);
  const [draftModel, setDraftModel] = useState<llm.ModelInfo>(emptyModel());

  // 自动发现
  const [discovering, setDiscovering] = useState(false);
  const [discoverError, setDiscoverError] = useState("");
  const [discoveredModels, setDiscoveredModels] = useState<llm.ModelInfo[]>([]);
  const [selectedForImport, setSelectedForImport] = useState<Set<string>>(
    new Set(),
  );
  const [pendingImports, setPendingImports] = useState<llm.ModelInfo[]>([]);

  const startAddForm = () => {
    setDraftModel(emptyModel());
    setShowAddForm(true);
  };

  const handleManualAdd = () => {
    onAddModel(draftModel);
    setShowAddForm(false);
  };

  const handleDiscover = async () => {
    setDiscovering(true);
    setDiscoverError("");
    setDiscoveredModels([]);
    setSelectedForImport(new Set());
    try {
      const models = await DiscoverModels(chatUrl, apiKey);
      if (!models || models.length === 0) {
        setDiscoverError(t("settings.noModelsFound"));
      } else {
        setDiscoveredModels(models);
        setSelectedForImport(new Set(models.map((m) => m.id)));
      }
    } catch (e: unknown) {
      setDiscoverError(toErrorMessage(e));
    } finally {
      setDiscovering(false);
    }
  };

  const handleImportSelected = () => {
    if (selectedForImport.size === 0) return;
    const toImport = discoveredModels.filter(
      (m) => selectedForImport.has(m.id) && !existingIds.has(m.id),
    );
    const withDefaults = toImport.map(
      (m) =>
        ({
          ...m,
          context_window: m.context_window || DEFAULT_CTX,
          max_output_tokens: m.max_output_tokens || DEFAULT_MAX,
          supports_thinking: true,
        }) as unknown as llm.ModelInfo,
    );
    setPendingImports(withDefaults);
    setDiscoveredModels([]);
    setSelectedForImport(new Set());
    setDiscoverError("");
  };

  const handleSavePending = (index: number, model: llm.ModelInfo) => {
    onAddModel(model);
    setPendingImports((prev) => prev.filter((_, i) => i !== index));
  };

  return (
    <div>
      <div className="flex items-center justify-between mb-2">
        <span className="text-xs text-muted-foreground">
          {t("settings.modelList")}
        </span>
        <div className="flex items-center gap-2">
          <button
            onClick={handleDiscover}
            disabled={discovering || !apiKey}
            className="text-xs text-primary flex items-center gap-0.5 hover:underline disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {discovering ? (
              <Loader2 className="w-3 h-3 animate-spin" />
            ) : (
              <Search className="w-3 h-3" />
            )}
            {t("settings.autoDiscover")}
          </button>
          <button
            onClick={startAddForm}
            className="text-xs text-primary flex items-center gap-0.5 hover:underline"
          >
            <Plus className="w-3 h-3" /> {t("settings.add")}
          </button>
        </div>
      </div>

      {/* 待导入可编辑卡片 */}
      {pendingImports.map((m, i) => (
        <div key={m.id + "_" + i} className="mb-2">
          <ModelEditForm
            model={m}
            onChange={(patch) =>
              setPendingImports((prev) =>
                prev.map((pm, j) =>
                  j === i ? ({ ...pm, ...patch } as llm.ModelInfo) : pm,
                ),
              )
            }
            onSave={() => handleSavePending(i, m)}
            onCancel={() =>
              setPendingImports((prev) => prev.filter((_, j) => j !== i))
            }
            title={t("settings.modelNumber", { n: i + 1 })}
          />
        </div>
      ))}

      {/* 手动添加表单 */}
      {showAddForm && (
        <div className="mb-2">
          <ModelEditForm
            model={draftModel}
            onChange={(patch) =>
              setDraftModel((prev) => ({ ...prev, ...patch }) as llm.ModelInfo)
            }
            onSave={handleManualAdd}
            onCancel={() => setShowAddForm(false)}
          />
        </div>
      )}

      {/* 发现错误（无结果时） */}
      {discoverError && !discovering && discoveredModels.length === 0 && (
        <TestResultWithHint
          testResult={{ ok: false, msg: discoverError }}
          className="mb-2 pl-0"
        />
      )}

      {/* 发现结果面板 */}
      {discoveredModels.length > 0 && (
        <div className="border rounded-md mb-2">
          <div className="flex items-center justify-between px-3 py-2 border-b">
            <span className="text-xs font-medium">
              {t("settings.foundModels", { count: discoveredModels.length })}
            </span>
            <div className="flex items-center gap-2">
              <button
                onClick={() => {
                  const allIds = discoveredModels.map((m) => m.id);
                  const allSelected = allIds.every((id) =>
                    selectedForImport.has(id),
                  );
                  setSelectedForImport(
                    allSelected ? new Set() : new Set(allIds),
                  );
                }}
                className="text-xs text-muted-foreground hover:underline"
              >
                {t("settings.selectAll")}
              </button>
              <button
                onClick={handleImportSelected}
                disabled={selectedForImport.size === 0}
                className="h-7 px-2.5 rounded-md bg-primary text-primary-foreground text-xs disabled:opacity-50"
              >
                {t("settings.importSelected")}
              </button>
              <button
                onClick={() => {
                  setDiscoveredModels([]);
                  setDiscoverError("");
                }}
                className="text-muted-foreground hover:text-destructive"
              >
                <X className="w-3.5 h-3.5" />
              </button>
            </div>
          </div>
          <div className="max-h-48 overflow-y-auto divide-y">
            {discoveredModels.map((m) => {
              const exists = existingIds.has(m.id);
              return (
                <label
                  key={m.id}
                  className={`flex items-center gap-2 px-3 py-1.5 text-xs cursor-pointer ${exists ? "opacity-50" : ""}`}
                >
                  <input
                    type="checkbox"
                    checked={selectedForImport.has(m.id)}
                    onChange={() => {
                      setSelectedForImport((prev) => {
                        const next = new Set(prev);
                        if (next.has(m.id)) next.delete(m.id);
                        else next.add(m.id);
                        return next;
                      });
                    }}
                    disabled={exists}
                    className="rounded shrink-0"
                  />
                  <span className="flex-1">{m.id}</span>
                  {m.context_window > 0 && (
                    <span className="text-muted-foreground">
                      {m.context_window >= 1_000_000
                        ? (m.context_window / 1_000_000).toFixed(0) + "M"
                        : (m.context_window / 1_000).toFixed(0) + "K"}
                    </span>
                  )}
                  {m.supports_thinking && (
                    <span className="text-muted-foreground">
                      {t("settings.thinking")}
                    </span>
                  )}
                  {m.supports_vision && (
                    <span className="text-muted-foreground">
                      {t("settings.vision")}
                    </span>
                  )}
                  {exists && (
                    <span className="text-muted-foreground">
                      {t("settings.alreadyAdded")}
                    </span>
                  )}
                </label>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}

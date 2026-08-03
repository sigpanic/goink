import { useState, useEffect, useMemo } from "react";
import { GitBranch, Pencil, Plus, Trash2, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useApp } from "@/hooks/useApp";
import { useRefresh } from "@/hooks/useRefresh";
import { useTheme } from "@/hooks/useTheme";
import { arcPalette } from "./arcColors";
import { storyarcKeys, arcNodeKeys, maxChapterKeys } from "@/lib/queryKeys";
import type { storyarc } from "@/hooks/useApp";
import StoryArcGraph from "@/components/storyarc/StoryArcGraph";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";
import AutoGrowTextarea from "@/components/ui/AutoGrowTextarea";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import { useFocusStore } from "@/stores/useFocusStore";
import { useStoryArcs } from "./useStoryArcs";
import { useArcNodes } from "./useArcNodes";
import { useMaxChapterNumber } from "./useMaxChapterNumber";

interface Props {
  novelId: number;
}

type ViewTab = "list" | "swimlane";

type Filter = "all" | "pending" | "completed" | "abandoned";
const WINDOW = 20;

const FILTERS: { key: Filter; label: string }[] = [
  { key: "all", label: "storyarc.all" },
  { key: "pending", label: "storyarc.inProgress" },
  { key: "completed", label: "storyarc.completed" },
  { key: "abandoned", label: "storyarc.abandoned" },
];

const ARC_TYPES = [
  { value: "main", label: "storyarc.mainline" },
  { value: "sub", label: "storyarc.subplot" },
  { value: "character", label: "storyarc.characterLine" },
  { value: "background", label: "storyarc.backgroundLine" },
];

const ARC_STATUSES = [
  { value: "active", label: "storyarc.active" },
  { value: "paused", label: "storyarc.paused" },
  { value: "completed", label: "storyarc.completed" },
  { value: "abandoned", label: "storyarc.abandoned" },
];

const NODE_STATUSES = [
  { value: "pending", label: "storyarc.inProgress" },
  { value: "completed", label: "storyarc.completed" },
  { value: "abandoned", label: "storyarc.abandoned" },
];

const IMPORTANCES = [1, 2, 3, 4, 5];
function stars(v: number) {
  return "★".repeat(Math.max(0, Math.min(5, v)));
}

type EditMode =
  | { type: "create_arc" }
  | { type: "edit_arc"; arc: storyarc.StoryArc }
  | { type: "create_node" }
  | { type: "edit_node"; node: storyarc.ArcNode }
  | null;

type ArcForm = {
  name: string;
  arc_type: string;
  description?: string;
  importance?: number;
  status?: string;
  reactivate_at?: string;
};
type NodeForm = {
  story_arc_id: number;
  title: string;
  description?: string;
  target_chapter: number;
  actual_chapter?: number;
  status?: string;
};

const EMPTY_ARC: ArcForm = { name: "", arc_type: "main" };
const EMPTY_NODE: NodeForm = { story_arc_id: 0, title: "", target_chapter: 1 };

export default function ArcListView({ novelId }: Props) {
  const focusArcId = useFocusStore((s) => s.focusMap.storyarcs ?? 0);
  const app = useApp();
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  const { theme } = useTheme();
  const { bumpRefresh, refreshNonce } = useRefresh();
  const PALETTE = arcPalette(theme);

  // 4.3.1: arcs/allNodes/maxChapter 走 query（与 ArcList / StoryArcGraph 共享缓存）。
  // 删原 useApp.GetStoryArcs/GetArcNodes/GetMaxChapterNumber + load() + useState；
  // CRUD 后由 bumpRefresh → refreshNonce → invalidateQueries 触发 refetch（commit 2/3 抽 mutation 后改 onSuccess invalidate）。
  // 4a: query 错误 toast 由全局中间件接管（queryErrorToast.ts），此处不再挂 useEffect。
  const arcsQuery = useStoryArcs(novelId);
  const nodesQuery = useArcNodes(novelId);
  const maxChQuery = useMaxChapterNumber(novelId);
  const arcs = arcsQuery.data ?? [];
  const allNodes = nodesQuery.data ?? [];
  const loading = arcsQuery.isLoading || nodesQuery.isLoading;
  // loadFailed 只看 arcs（arcs 失败整列表不可用）。nodes 失败时列表仍渲染（空节点）+ toast。
  const loadFailed = arcsQuery.isError;

  const [expandedId, setExpandedId] = useState<number | null>(null);
  const [windowCenter, setWindowCenter] = useState(0);
  const [filter, setFilter] = useState<Filter>("all");
  const [hiddenArcIds, setHiddenArcIds] = useState<Set<number>>(new Set());
  const [viewTab, setViewTab] = useState<ViewTab>("list");
  const [editMode, setEditMode] = useState<EditMode>(null);
  const [arcForm, setArcForm] = useState<ArcForm>(EMPTY_ARC);
  const [nodeForm, setNodeForm] = useState<NodeForm>(EMPTY_NODE);
  const [saving, setSaving] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<{
    kind: "arc" | "node";
    id: number;
  } | null>(null);
  const [deleting, setDeleting] = useState(false);

  // 4.3.1: maxChapter 就绪后初始化 windowCenter（替代原 load() 里的 setWindowCenter）。
  useEffect(() => {
    const max = maxChQuery.data ?? 0;
    if (max > 0) setWindowCenter(Math.max(1, max));
  }, [maxChQuery.data]);

  // 4.3.1: refreshNonce 变化时 invalidate 三个 query（替代原 load()）。
  // CRUD 后 bumpRefresh 触发 refreshNonce → invalidate → query refetch。
  // commit 2/3 抽 mutation 后改 onSuccess invalidate，届时删 bumpRefresh + 此 effect。
  useEffect(() => {
    if (!refreshNonce) return;
    queryClient.invalidateQueries({ queryKey: storyarcKeys.list(novelId) });
    queryClient.invalidateQueries({ queryKey: arcNodeKeys.list(novelId) });
    queryClient.invalidateQueries({ queryKey: maxChapterKeys.detail(novelId) });
  }, [refreshNonce, queryClient, novelId]);

  useEffect(() => {
    if (focusArcId && focusArcId > 0 && allNodes.length > 0) {
      const arcNodes = allNodes.filter((n) => n.story_arc_id === focusArcId);
      if (arcNodes.length > 0) {
        const firstNode = arcNodes[0];
        setWindowCenter(
          firstNode.target_chapter || firstNode.actual_chapter || 1,
        );
        setExpandedId(firstNode.id);
      }
    }
  }, [focusArcId, allNodes]);

  const windowFrom = Math.max(1, windowCenter - WINDOW);
  const windowTo = windowCenter + WINDOW;

  const activeArcIds = useMemo(() => {
    if (hiddenArcIds.size === 0) return new Set(arcs.map((a) => a.id));
    return new Set(arcs.map((a) => a.id).filter((id) => !hiddenArcIds.has(id)));
  }, [arcs, hiddenArcIds]);

  const filteredNodes = useMemo(() => {
    let nodes = allNodes.filter((n) => activeArcIds.has(n.story_arc_id));
    if (filter !== "all") nodes = nodes.filter((n) => n.status === filter);
    return nodes;
  }, [allNodes, activeArcIds, filter]);

  const grouped = useMemo(() => {
    const map = new Map<number, storyarc.ArcNode[]>();
    for (const n of filteredNodes) {
      const ch = n.target_chapter;
      if (!map.has(ch)) map.set(ch, []);
      map.get(ch)!.push(n);
    }
    return [...map.entries()].sort(([a], [b]) => a - b);
  }, [filteredNodes]);

  const visibleChapters = grouped.filter(
    ([ch]) => ch >= windowFrom && ch <= windowTo,
  );
  const beforeChapters = grouped.filter(([ch]) => ch < windowFrom);
  const afterChapters = grouped.filter(([ch]) => ch > windowTo);
  const beforeCount = beforeChapters.reduce(
    (s, [, items]) => s + items.length,
    0,
  );
  const afterCount = afterChapters.reduce(
    (s, [, items]) => s + items.length,
    0,
  );
  const maxChapter = grouped.length > 0 ? grouped[grouped.length - 1][0] : 0;

  function shiftWindow(delta: number) {
    setWindowCenter((prev) =>
      Math.max(WINDOW + 1, Math.min(maxChapter - WINDOW, prev + delta)),
    );
  }

  function toggleArc(id: number) {
    setHiddenArcIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function showAllArcs() {
    setHiddenArcIds(new Set());
  }

  // ── Arc CRUD ─────────────────────────────────────────

  function openCreateArc() {
    setArcForm(EMPTY_ARC);
    setEditMode({ type: "create_arc" });
  }

  function openEditArc(arc: storyarc.StoryArc) {
    setArcForm({
      name: arc.name,
      arc_type: arc.arc_type,
      description: arc.description || "",
      importance: arc.importance,
    });
    setEditMode({ type: "edit_arc", arc });
  }

  async function handleCreateArc() {
    if (!arcForm.name.trim()) {
      toastError(t("storyarc.pleaseEnterArcName"));
      return;
    }
    if (!arcForm.arc_type) {
      toastError(t("storyarc.pleaseSelectArcType"));
      return;
    }
    setSaving(true);
    try {
      await app.CreateStoryArc(novelId, arcForm);
      setEditMode(null);
      bumpRefresh();
    } catch (err) {
      toastError(t("storyarc.createArcFailed") + ": " + toErrorMessage(err));
      console.error(err);
    } finally {
      setSaving(false);
    }
  }

  async function handleUpdateArc() {
    if (!editMode || editMode.type !== "edit_arc") return;
    setSaving(true);
    try {
      await app.UpdateStoryArc(novelId, editMode.arc.id, arcForm);
      setEditMode(null);
      bumpRefresh();
    } catch (err) {
      toastError(t("storyarc.updateArcFailed") + ": " + toErrorMessage(err));
      console.error(err);
    } finally {
      setSaving(false);
    }
  }

  function handleDeleteArc(arcId: number) {
    setDeleteTarget({ kind: "arc", id: arcId });
  }

  // ── Node CRUD ────────────────────────────────────────

  function openCreateNode(arcId?: number) {
    setNodeForm({
      ...EMPTY_NODE,
      story_arc_id: arcId ?? arcs[0]?.id ?? 0,
      target_chapter: Math.max(1, windowCenter),
    });
    setEditMode({ type: "create_node" });
  }

  function openEditNode(node: storyarc.ArcNode) {
    setNodeForm({
      story_arc_id: node.story_arc_id,
      title: node.title,
      description: node.description || "",
      target_chapter: node.target_chapter,
    });
    setEditMode({ type: "edit_node", node });
  }

  async function handleCreateNode() {
    if (!nodeForm.title.trim()) {
      toastError(t("storyarc.pleaseEnterNodeTitle"));
      return;
    }
    if (!nodeForm.story_arc_id) {
      toastError(t("storyarc.pleaseSelectParentArc"));
      return;
    }
    if (!nodeForm.target_chapter) {
      toastError(t("storyarc.pleaseEnterTargetChapter"));
      return;
    }
    setSaving(true);
    try {
      const created = await app.CreateArcNode(novelId, nodeForm);
      setEditMode(null);
      // 4.3.1: load() 已删，CRUD 后由 bumpRefresh → invalidateQueries 刷新。
      setExpandedId(created.id);
      bumpRefresh();
    } catch (err) {
      toastError(t("storyarc.createNodeFailed") + ": " + toErrorMessage(err));
      console.error(err);
    } finally {
      setSaving(false);
    }
  }

  async function handleUpdateNode() {
    if (!editMode || editMode.type !== "edit_node") return;
    if (!nodeForm.title.trim()) {
      toastError(t("storyarc.pleaseEnterNodeTitle"));
      return;
    }
    const nodeId = editMode.node.id;
    setSaving(true);
    try {
      await app.UpdateArcNode(novelId, nodeId, nodeForm);
      setEditMode(null);
      // 4.3.1: load() 已删，CRUD 后由 bumpRefresh → invalidateQueries 刷新。
      setExpandedId(nodeId);
      bumpRefresh();
    } catch (err) {
      toastError(t("storyarc.updateNodeFailed") + ": " + toErrorMessage(err));
      console.error(err);
    } finally {
      setSaving(false);
    }
  }

  function handleDeleteNode(nodeId: number) {
    setDeleteTarget({ kind: "node", id: nodeId });
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      if (deleteTarget.kind === "arc") {
        await app.DeleteStoryArc(novelId, deleteTarget.id);
        setExpandedId(null);
      } else {
        await app.DeleteArcNode(novelId, deleteTarget.id);
        setExpandedId(null);
      }
      setDeleteTarget(null);
      bumpRefresh();
    } catch (err) {
      const key =
        deleteTarget.kind === "arc"
          ? "storyarc.deleteArcFailed"
          : "storyarc.deleteNodeFailed";
      toastError(t(key) + ": " + toErrorMessage(err));
      console.error(err);
    } finally {
      setDeleting(false);
    }
  }

  async function handleQuickNodeStatus(
    node: storyarc.ArcNode,
    newStatus: string,
  ) {
    setSaving(true);
    try {
      await app.UpdateArcNode(novelId, node.id, { status: newStatus });
      bumpRefresh();
    } catch (err) {
      toastError(
        t("storyarc.updateNodeStatusFailed") + ": " + toErrorMessage(err),
      );
      console.error(err);
    } finally {
      setSaving(false);
    }
  }

  const nodeStatusStyle = (status: string) => {
    switch (status) {
      case "completed":
        return {
          bg: "bg-tag-green",
          text: "text-tag-green-foreground",
          label: t("storyarc.completed"),
        };
      case "abandoned":
        return {
          bg: "bg-secondary",
          text: "text-muted-foreground",
          label: t("storyarc.abandoned"),
        };
      default:
        return {
          bg: "bg-tag-blue",
          text: "text-tag-blue-foreground",
          label: t("storyarc.inProgress"),
        };
    }
  };

  const arcStatusTag = (status: string) => {
    switch (status) {
      case "paused":
        return " ⏸";
      case "completed":
        return " ✓";
      case "abandoned":
        return " ✗";
      default:
        return "";
    }
  };

  function renderArcForm(isCreate: boolean) {
    return (
      <div className="space-y-3">
        <div>
          <label className="text-xs font-medium text-muted-foreground mb-1 block">
            {t("storyarc.name")}
          </label>
          <input
            type="text"
            value={arcForm.name}
            onChange={(e) =>
              setArcForm((f) => ({ ...f, name: e.target.value }))
            }
            className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            placeholder={t("storyarc.arcName")}
          />
        </div>
        <div className="flex gap-3">
          <div className="flex-1">
            <label className="text-xs font-medium text-muted-foreground mb-1 block">
              {t("storyarc.type")}
            </label>
            <select
              value={arcForm.arc_type}
              onChange={(e) =>
                setArcForm((f) => ({ ...f, arc_type: e.target.value }))
              }
              className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {ARC_TYPES.map((opt) => (
                <option key={opt.value} value={opt.value}>
                  {t(opt.label)}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="text-xs font-medium text-muted-foreground mb-1 block">
              {t("storyarc.importance")}
            </label>
            <select
              value={arcForm.importance}
              onChange={(e) =>
                setArcForm((f) => ({
                  ...f,
                  importance: parseInt(e.target.value),
                }))
              }
              className="rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {IMPORTANCES.map((i) => (
                <option key={i} value={i}>
                  {stars(i)}
                </option>
              ))}
            </select>
          </div>
        </div>
        <div>
          <label className="text-xs font-medium text-muted-foreground mb-1 block">
            {t("storyarc.description")}
          </label>
          <AutoGrowTextarea
            value={arcForm.description}
            onChange={(e) =>
              setArcForm((f) => ({ ...f, description: e.target.value }))
            }
            minHeight={40}
            maxHeight={160}
            className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            placeholder={t("storyarc.arcDescription")}
          />
        </div>
        {!isCreate && editMode?.type === "edit_arc" && (
          <div>
            <label className="text-xs font-medium text-muted-foreground mb-1 block">
              {t("storyarc.status")}
            </label>
            <select
              value={arcForm.status ?? editMode.arc.status}
              onChange={(e) =>
                setArcForm((f) => ({ ...f, status: e.target.value }))
              }
              className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {ARC_STATUSES.map((opt) => (
                <option key={opt.value} value={opt.value}>
                  {t(opt.label)}
                </option>
              ))}
            </select>
          </div>
        )}
      </div>
    );
  }

  function renderNodeForm() {
    return (
      <div className="space-y-3">
        {editMode?.type === "create_node" && (
          <div>
            <label className="text-xs font-medium text-muted-foreground mb-1 block">
              {t("storyarc.parentArc")}
            </label>
            <select
              value={nodeForm.story_arc_id}
              onChange={(e) =>
                setNodeForm((f) => ({
                  ...f,
                  story_arc_id: parseInt(e.target.value),
                }))
              }
              className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {arcs.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </select>
          </div>
        )}
        <div>
          <label className="text-xs font-medium text-muted-foreground mb-1 block">
            {t("storyarc.title")}
          </label>
          <input
            type="text"
            value={nodeForm.title}
            onChange={(e) =>
              setNodeForm((f) => ({ ...f, title: e.target.value }))
            }
            className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            placeholder={t("storyarc.nodeTitle")}
          />
        </div>
        <div>
          <label className="text-xs font-medium text-muted-foreground mb-1 block">
            {t("storyarc.description")}
          </label>
          <AutoGrowTextarea
            value={nodeForm.description}
            onChange={(e) =>
              setNodeForm((f) => ({ ...f, description: e.target.value }))
            }
            minHeight={40}
            maxHeight={160}
            className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            placeholder={t("storyarc.nodeDetails")}
          />
        </div>
        <div className="flex gap-3">
          <div className="flex-1">
            <label className="text-xs font-medium text-muted-foreground mb-1 block">
              {t("storyarc.targetChapter")}
            </label>
            <input
              type="number"
              value={nodeForm.target_chapter}
              onChange={(e) =>
                setNodeForm((f) => ({
                  ...f,
                  target_chapter: parseInt(e.target.value) || 1,
                }))
              }
              min={1}
              className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>
          {editMode?.type === "edit_node" && (
            <div>
              <label className="text-xs font-medium text-muted-foreground mb-1 block">
                {t("storyarc.status")}
              </label>
              <select
                value={nodeForm.status ?? editMode.node.status}
                onChange={(e) =>
                  setNodeForm((f) => ({ ...f, status: e.target.value }))
                }
                className="rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                {NODE_STATUSES.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {t(opt.label)}
                  </option>
                ))}
              </select>
            </div>
          )}
        </div>
      </div>
    );
  }

  function renderFormButtons(
    onSubmit: () => Promise<void>,
    onDelete?: () => void,
  ) {
    return (
      <div className="flex items-center gap-2 justify-end mt-3">
        {onDelete && (
          <button
            onClick={onDelete}
            disabled={saving}
            className="px-3 py-1 rounded text-xs text-destructive hover:bg-destructive/10 transition-colors"
          >
            <Trash2 className="h-3 w-3 inline mr-1" />
            {t("storyarc.delete")}
          </button>
        )}
        <button
          onClick={() => setEditMode(null)}
          className="px-3 py-1 rounded text-xs text-muted-foreground hover:text-foreground transition-colors"
        >
          {t("storyarc.cancel")}
        </button>
        <button
          onClick={onSubmit}
          disabled={saving}
          className="px-3 py-1 rounded bg-primary text-primary-foreground text-xs font-medium hover:opacity-90 transition-opacity disabled:opacity-50"
        >
          {saving ? t("storyarc.saving") : t("storyarc.save")}
        </button>
      </div>
    );
  }

  return (
    <main className="flex-1 min-w-0 flex flex-col overflow-hidden bg-background">
      {/* Tab bar */}
      <div className="flex items-center gap-1 px-5 pt-4 pb-2 shrink-0">
        <button
          onClick={() => setViewTab("list")}
          className={`px-3 py-1.5 rounded text-xs font-medium transition-colors ${
            viewTab === "list"
              ? "bg-card border border-border text-foreground shadow-sm"
              : "text-muted-foreground hover:text-foreground hover:bg-card/60"
          }`}
        >
          {t("storyarc.list")}
        </button>
        <button
          onClick={() => setViewTab("swimlane")}
          className={`px-3 py-1.5 rounded text-xs font-medium transition-colors ${
            viewTab === "swimlane"
              ? "bg-card border border-border text-foreground shadow-sm"
              : "text-muted-foreground hover:text-foreground hover:bg-card/60"
          }`}
        >
          {t("storyarc.swimlane")}
        </button>
      </div>

      {viewTab === "swimlane" ? (
        <StoryArcGraph novelId={novelId} />
      ) : loading ? (
        <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
          {t("storyarc.loading")}
        </div>
      ) : (
        <div className="flex-1 overflow-y-auto overscroll-contain">
          <div className="max-w-3xl mx-auto px-5 py-6 space-y-6">
            {/* Header */}
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <GitBranch className="h-4 w-4 text-tag-purple-foreground" />
                <h2 className="text-sm font-semibold text-foreground">
                  {t("storyarc.arcNode")}
                  <span className="ml-2 text-xs font-normal text-muted-foreground">
                    {filteredNodes.length} {t("storyarc.countUnit")}
                  </span>
                </h2>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-[11px] text-muted-foreground">
                  {t("sidebar.chapterRange", {
                    start: windowFrom,
                    end: windowTo,
                  })}{" "}
                  · {t("storyarc.totalChapters", { count: maxChapter })}
                </span>
                <button
                  onClick={() => {
                    // 4.3.1: refresh 按钮 invalidate 三个 query（替代原 load()）。
                    queryClient.invalidateQueries({ queryKey: storyarcKeys.list(novelId) });
                    queryClient.invalidateQueries({ queryKey: arcNodeKeys.list(novelId) });
                    queryClient.invalidateQueries({ queryKey: maxChapterKeys.detail(novelId) });
                  }}
                  className="text-xs text-muted-foreground hover:text-muted-foreground transition-colors"
                >
                  {t("storyarc.refresh")}
                </button>
              </div>
            </div>

            {/* Arc filter chips */}
            <div className="flex flex-wrap gap-1.5">
              <button
                onClick={showAllArcs}
                className={`px-3 py-1 rounded text-xs font-medium transition-colors ${
                  hiddenArcIds.size === 0
                    ? "bg-card border border-border text-foreground shadow-sm"
                    : "text-muted-foreground hover:text-foreground hover:bg-card/60"
                }`}
              >
                {t("storyarc.all")}
              </button>
              {arcs.map((arc, i) => {
                const c = PALETTE[i % PALETTE.length];
                const hidden = hiddenArcIds.has(arc.id);
                return (
                  <button
                    key={arc.id}
                    onClick={() => toggleArc(arc.id)}
                    className={`group px-3 py-1 rounded text-xs font-medium transition-colors border relative ${
                      hidden
                        ? "text-muted-foreground border-transparent hover:text-muted-foreground hover:bg-card/60"
                        : "border-border shadow-sm text-foreground"
                    }`}
                    style={
                      hidden
                        ? {}
                        : {
                            backgroundColor: c.fill,
                            borderColor: c.stroke,
                            color: c.text,
                          }
                    }
                  >
                    {arc.name}
                    {arcStatusTag(arc.status)}
                    {/* Hover actions */}
                    <span
                      className="ml-1 opacity-0 group-hover:opacity-100 inline-flex items-center gap-1 transition-opacity"
                      style={{ color: hidden ? undefined : c.text }}
                    >
                      <span
                        onClick={(e) => {
                          e.stopPropagation();
                          openEditArc(arc);
                        }}
                        className="p-0.5 rounded hover:opacity-70"
                        title={t("storyarc.edit")}
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </span>
                      <span
                        onClick={(e) => {
                          e.stopPropagation();
                          handleDeleteArc(arc.id);
                        }}
                        className="p-0.5 rounded hover:opacity-70"
                        title={t("storyarc.delete")}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </span>
                    </span>
                  </button>
                );
              })}
              <button
                onClick={openCreateArc}
                className="px-3 py-1 rounded text-xs font-medium text-muted-foreground hover:text-foreground hover:bg-card/60 transition-colors border border-dashed border-border"
              >
                <Plus className="h-3 w-3 inline mr-1" />
                {t("storyarc.newArc2")}
              </button>
            </div>

            {/* Arc form */}
            {editMode?.type === "create_arc" && (
              <div className="rounded-lg border border-border bg-card p-4">
                <div className="flex items-center justify-between mb-3">
                  <span className="text-xs font-semibold text-foreground">
                    {t("storyarc.newArc")}
                  </span>
                  <button
                    onClick={() => setEditMode(null)}
                    className="p-0.5 rounded text-muted-foreground hover:text-foreground"
                  >
                    <X className="h-3.5 w-3.5" />
                  </button>
                </div>
                {renderArcForm(true)}
                {renderFormButtons(handleCreateArc)}
              </div>
            )}
            {editMode?.type === "edit_arc" && (
              <div className="rounded-lg border border-border bg-card p-4">
                <div className="flex items-center justify-between mb-3">
                  <span className="text-xs font-semibold text-foreground">
                    {t("storyarc.editArc")}
                    {editMode.arc.name}
                  </span>
                  <button
                    onClick={() => setEditMode(null)}
                    className="p-0.5 rounded text-muted-foreground hover:text-foreground"
                  >
                    <X className="h-3.5 w-3.5" />
                  </button>
                </div>
                {renderArcForm(false)}
                {renderFormButtons(handleUpdateArc, () =>
                  handleDeleteArc(editMode.arc.id),
                )}
              </div>
            )}

            {/* Quick actions bar */}
            <div className="flex items-center justify-between">
              <div className="flex gap-1">
                {FILTERS.map((f) => (
                  <button
                    key={f.key}
                    onClick={() => setFilter(f.key)}
                    className={`px-3 py-1 rounded text-xs transition-colors ${
                      filter === f.key
                        ? "bg-card border border-border text-foreground shadow-sm"
                        : "text-muted-foreground hover:text-foreground"
                    }`}
                  >
                    {t(f.label)}
                    {f.key !== "all" && (
                      <span className="ml-1 text-muted-foreground">
                        (
                        {
                          allNodes.filter(
                            (n) =>
                              activeArcIds.has(n.story_arc_id) &&
                              n.status === f.key,
                          ).length
                        }
                        )
                      </span>
                    )}
                  </button>
                ))}
              </div>
              {arcs.length > 0 && (
                <button
                  onClick={() => openCreateNode()}
                  className="inline-flex items-center gap-1 px-2.5 py-1 rounded text-xs font-medium bg-primary text-primary-foreground hover:opacity-90 transition-opacity"
                >
                  <Plus className="h-3 w-3" />
                  {t("storyarc.newNode")}
                </button>
              )}
            </div>

            {/* Node form */}
            {editMode?.type === "create_node" && (
              <div className="rounded-lg border border-border bg-card p-4">
                <div className="flex items-center justify-between mb-3">
                  <span className="text-xs font-semibold text-foreground">
                    {t("storyarc.newNode")}
                  </span>
                  <button
                    onClick={() => setEditMode(null)}
                    className="p-0.5 rounded text-muted-foreground hover:text-foreground"
                  >
                    <X className="h-3.5 w-3.5" />
                  </button>
                </div>
                {renderNodeForm()}
                {renderFormButtons(handleCreateNode)}
              </div>
            )}

            {/* Node list */}
            {loadFailed ? (
              <p className="text-xs text-destructive py-4">
                {t("storyarc.arcsLoadFailed")}
              </p>
            ) : grouped.length === 0 ? (
              <div className="text-center py-12">
                <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-tag-purple text-tag-purple-foreground">
                  <GitBranch className="h-5 w-5" />
                </div>
                <p className="mt-2 text-sm text-muted-foreground">
                  {arcs.length === 0
                    ? t("storyarc.noNarrativeArcs")
                    : t("storyarc.noMatchingNodes")}
                </p>
              </div>
            ) : (
              <div className="space-y-6">
                {beforeCount > 0 && (
                  <button
                    onClick={() => shiftWindow(-WINDOW)}
                    className="w-full rounded-lg border border-dashed border-border bg-card/60 px-4 py-2.5 text-xs text-muted-foreground hover:bg-card hover:border-border hover:text-foreground transition-colors"
                  >
                    ←{" "}
                    {t("storyarc.earlierChapters", {
                      start: beforeChapters[0]?.[0],
                      end: beforeChapters[beforeChapters.length - 1]?.[0],
                    })}{" "}
                    · {t("storyarc.nodeCount", { count: beforeCount })}
                  </button>
                )}

                {visibleChapters.map(([ch, items]) => (
                  <div key={ch}>
                    <div className="flex items-center gap-1.5 mb-2">
                      <span className="text-xs font-medium text-muted-foreground">
                        {t("sidebar.chapterN", { n: ch })}
                      </span>
                      <span className="text-[10px] text-muted-foreground">
                        {t("storyarc.nodeCount", { count: items.length })}
                      </span>
                    </div>
                    <div className="space-y-2">
                      {items.map((node) => {
                        const s = nodeStatusStyle(node.status);
                        const arcIdx = arcs.findIndex(
                          (a) => a.id === node.story_arc_id,
                        );
                        const c =
                          PALETTE[arcIdx >= 0 ? arcIdx % PALETTE.length : 0];
                        const arc = arcIdx >= 0 ? arcs[arcIdx] : null;
                        const isExpanded = expandedId === node.id;
                        const desc = node.description?.trim() || "";
                        const hasContent = desc.length > 0;
                        const isEditing =
                          editMode?.type === "edit_node" &&
                          editMode.node.id === node.id;

                        if (isEditing) {
                          return (
                            <div
                              key={node.id}
                              className="rounded-lg border border-border bg-card p-4"
                            >
                              <div className="flex items-center justify-between mb-3">
                                <span className="text-xs font-semibold text-foreground">
                                  {t("storyarc.editing")}
                                  {node.title}
                                </span>
                                <button
                                  onClick={() => setEditMode(null)}
                                  className="p-0.5 rounded text-muted-foreground hover:text-foreground"
                                >
                                  <X className="h-3.5 w-3.5" />
                                </button>
                              </div>
                              {renderNodeForm()}
                              {renderFormButtons(handleUpdateNode, () =>
                                handleDeleteNode(node.id),
                              )}
                            </div>
                          );
                        }

                        return (
                          <div
                            key={node.id}
                            className={`rounded-lg border bg-card transition-shadow group ${isExpanded ? "border-border shadow-sm" : "border-border hover:border-border hover:shadow-sm"}`}
                          >
                            <div className="flex items-center gap-3 px-4 py-3">
                              <span
                                className="shrink-0 h-3 w-3 rounded-full"
                                style={{ backgroundColor: c.stroke }}
                              />
                              <div
                                className="flex-1 min-w-0 cursor-pointer"
                                onClick={() =>
                                  setExpandedId(isExpanded ? null : node.id)
                                }
                              >
                                <div className="flex items-center gap-2">
                                  <span className="text-sm font-medium text-foreground truncate">
                                    {node.title}
                                  </span>
                                  <span
                                    className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium ${s.bg} ${s.text}`}
                                  >
                                    {s.label}
                                  </span>
                                  {arc && (
                                    <span
                                      className="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium"
                                      style={{
                                        backgroundColor: c.fill,
                                        color: c.text,
                                      }}
                                    >
                                      {arc.name}
                                    </span>
                                  )}
                                </div>
                                <div className="flex items-center gap-2 mt-0.5 text-[11px] text-muted-foreground">
                                  <span>
                                    {t("storyarc.targetChapterN", {
                                      n: node.target_chapter,
                                    })}
                                  </span>
                                  {node.actual_chapter > 0 && (
                                    <span className="text-tag-green-foreground">
                                      ·{" "}
                                      {t("storyarc.actualChapterN", {
                                        n: node.actual_chapter,
                                      })}
                                    </span>
                                  )}
                                  {arc && (
                                    <span className="text-muted-foreground">
                                      · {arc.arc_type}
                                    </span>
                                  )}
                                </div>
                              </div>
                              {/* Hover actions */}
                              <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                                {node.status === "pending" && (
                                  <button
                                    onClick={() =>
                                      handleQuickNodeStatus(node, "completed")
                                    }
                                    className="p-1 rounded text-muted-foreground hover:text-tag-green-foreground hover:bg-tag-green/20 transition-colors"
                                    title={t("storyarc.markComplete")}
                                  >
                                    <span className="text-[10px]">✓</span>
                                  </button>
                                )}
                                <button
                                  onClick={() => openEditNode(node)}
                                  className="p-1 rounded text-muted-foreground hover:text-foreground hover:bg-secondary transition-colors"
                                  title={t("storyarc.edit")}
                                >
                                  <Pencil className="h-3.5 w-3.5" />
                                </button>
                                <button
                                  onClick={() => handleDeleteNode(node.id)}
                                  className="p-1 rounded text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors"
                                  title={t("storyarc.delete")}
                                >
                                  <Trash2 className="h-3.5 w-3.5" />
                                </button>
                              </div>
                              <span
                                className={`text-[10px] transition-transform cursor-pointer ${isExpanded ? "rotate-180" : ""}`}
                                onClick={() =>
                                  setExpandedId(isExpanded ? null : node.id)
                                }
                              >
                                ▼
                              </span>
                            </div>

                            {isExpanded && hasContent && (
                              <div className="border-t border-border px-4 py-3">
                                <p className="text-xs text-muted-foreground leading-relaxed whitespace-pre-wrap">
                                  {desc}
                                </p>
                              </div>
                            )}
                            {isExpanded && !hasContent && (
                              <div className="border-t border-border px-4 py-3">
                                <p className="text-xs text-muted-foreground">
                                  {t("storyarc.noDetailDescription")}
                                </p>
                              </div>
                            )}
                          </div>
                        );
                      })}
                    </div>
                  </div>
                ))}

                {afterCount > 0 && (
                  <button
                    onClick={() => shiftWindow(WINDOW)}
                    className="w-full rounded-lg border border-dashed border-border bg-card/60 px-4 py-2.5 text-xs text-muted-foreground hover:bg-card hover:border-border hover:text-foreground transition-colors"
                  >
                    →{" "}
                    {t("storyarc.laterChapters", {
                      start: afterChapters[0]?.[0],
                      end: afterChapters[afterChapters.length - 1]?.[0],
                    })}{" "}
                    · {t("storyarc.nodeCount", { count: afterCount })}
                  </button>
                )}
              </div>
            )}
          </div>
        </div>
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title={t("common.confirmDelete")}
        message={
          deleteTarget?.kind === "arc"
            ? t("storyarc.confirmDeleteArc")
            : deleteTarget?.kind === "node"
              ? t("storyarc.confirmDeleteNode")
              : ""
        }
        danger
        loading={deleting}
        confirmText={t("common.delete")}
        onClose={() => setDeleteTarget(null)}
        onConfirm={confirmDelete}
      />
    </main>
  );
}

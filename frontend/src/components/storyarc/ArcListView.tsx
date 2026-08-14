import { useState, useEffect, useMemo } from "react";
import { GitBranch, Pencil, Plus, Trash2, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useThemeStore } from "@/stores/useThemeStore";
import { arcPalette } from "./arcColors";
import { storyarcKeys, arcNodeKeys, maxChapterKeys } from "@/lib/queryKeys";
import type { storyarc } from "@/lib/wailsjs/go/models";
import StoryArcGraph from "@/components/storyarc/StoryArcGraph";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";
import AutoGrowTextarea from "@/components/ui/AutoGrowTextarea";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import { useFocusWithNonce } from "@/hooks/useFocusWithNonce";
import { useStoryArcs } from "./useStoryArcs";
import { useArcNodes } from "./useArcNodes";
import { useMaxChapterNumber } from "./useMaxChapterNumber";
import { useDeleteStoryArc } from "./useDeleteStoryArc";
import { useDeleteArcNode } from "./useDeleteArcNode";
import { useCreateStoryArc } from "./useCreateStoryArc";
import { useUpdateStoryArc } from "./useUpdateStoryArc";
import { useCreateArcNode } from "./useCreateArcNode";
import { useUpdateArcNode } from "./useUpdateArcNode";

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

// 4b: ARC_TYPES 只存数据值，label 由 t("storyarc." + value) 动态拼接（与搜索 subtitle 同机制）。
const ARC_TYPES = ["main", "sub", "character", "background"];

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
  description: string;
  importance: number;
  status: string;
};
type NodeForm = {
  story_arc_id: number;
  title: string;
  description: string;
  target_chapter: number;
  actual_chapter: number;
  status: string;
};

const EMPTY_ARC: ArcForm = { name: "", arc_type: "main", description: "", importance: 1, status: "" };
const EMPTY_NODE: NodeForm = { story_arc_id: 0, title: "", description: "", target_chapter: 1, actual_chapter: 0, status: "pending" };

export default function ArcListView({ novelId }: Props) {
  const focus = useFocusWithNonce("storyarcs");
  const focusId = focus?.id ?? 0;
  const focusType = focus?.type; // "arc" | "node" | undefined
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  const { theme } = useThemeStore();
  const PALETTE = arcPalette(theme);

  // 4.3.1: arcs/allNodes/maxChapter 走 query（与 ArcList / StoryArcGraph 共享缓存）。
  // 删原 useApp.GetStoryArcs/GetArcNodes/GetMaxChapterNumber + load() + useState；
  // CRUD 后由 mutation onSuccess invalidate 触发 refetch（4.3.2/4.3.3）。
  // 4a: query 错误 toast 由全局中间件接管（queryErrorToast.ts），此处不再挂 useEffect。
  const arcsQuery = useStoryArcs(novelId);
  const nodesQuery = useArcNodes(novelId);
  const maxChQuery = useMaxChapterNumber(novelId);
  const arcs = arcsQuery.data ?? [];
  const allNodes = nodesQuery.data ?? [];
  const loading = arcsQuery.isLoading || nodesQuery.isLoading;
  // loadFailed 只看 arcs（arcs 失败整列表不可用）。nodes 失败时列表仍渲染（空节点）+ toast。
  const loadFailed = arcsQuery.isError;

  // 4.3.2/4.3.3: CRUD 走 mutation，deleting/saving 由 mutation.isPending 推导（不再用 useState）。
  // onSuccess 失效对应 query（删/改 arc 失效 storyarcs；删/改 node 失效 arc-nodes；
  // 删 arc 级联删其下 node，故额外失效 arc-nodes；create arc 无 node 不失效 arc-nodes）。
  const deleteArcMutation = useDeleteStoryArc(novelId);
  const deleteNodeMutation = useDeleteArcNode(novelId);
  const createArcMutation = useCreateStoryArc(novelId);
  const updateArcMutation = useUpdateStoryArc(novelId);
  const createNodeMutation = useCreateArcNode(novelId);
  const updateNodeMutation = useUpdateArcNode(novelId);
  const deleting = deleteArcMutation.isPending || deleteNodeMutation.isPending;
  const saving =
    createArcMutation.isPending ||
    updateArcMutation.isPending ||
    createNodeMutation.isPending ||
    updateNodeMutation.isPending;

  const [expandedId, setExpandedId] = useState<number | null>(null);
  const [windowCenter, setWindowCenter] = useState(0);
  const [filter, setFilter] = useState<Filter>("all");
  const [hiddenArcIds, setHiddenArcIds] = useState<Set<number>>(new Set());
  const [viewTab, setViewTab] = useState<ViewTab>("list");
  const [editMode, setEditMode] = useState<EditMode>(null);
  const [arcForm, setArcForm] = useState<ArcForm>(EMPTY_ARC);
  const [nodeForm, setNodeForm] = useState<NodeForm>(EMPTY_NODE);
  const [deleteTarget, setDeleteTarget] = useState<{
    kind: "arc" | "node";
    id: number;
  } | null>(null);
  // 4b: node 高亮声明式——focus type=node 触发后由 state 驱动 className（参考 CharacterListView highlightedId）。
  const [highlightedNodeId, setHighlightedNodeId] = useState<number | null>(
    null,
  );

  // 4.3.1: maxChapter 就绪后初始化 windowCenter（替代原 load() 里的 setWindowCenter）。
  useEffect(() => {
    const max = maxChQuery.data ?? 0;
    if (max > 0) setWindowCenter(Math.max(1, max));
  }, [maxChQuery.data]);

  // 4b: soloArc——只看目标 arc，隐藏其他所有 arc（等价用户手动逐条 toggleArc 隐藏其他）。
  // 点击搜索结果 arc/node 都会触发，过滤只看这一条弧线的 node。
  function soloArc(arcId: number) {
    setHiddenArcIds(
      new Set(arcs.filter((a) => a.id !== arcId).map((a) => a.id)),
    );
  }

  // 4b: focus 按 type 分流定位（arc→过滤+窗口右边界对齐 arc 末节点章节+展开首节点；node→过滤+高亮+窗口对齐node章节+展开）。
  useEffect(() => {
    if (!focus || focusId <= 0) return;

    if (focusType === "arc") {
      // 点击 arc 条目 → soloArc 只看这条弧线 + 窗口右边界对齐到 arc 末节点章节 + 展开首节点
      // 注：右边界=windowCenter+WINDOW，故 windowCenter = maxChapterOfArc - WINDOW；
      // arc 没节点才 fallback 到全书 maxChapter。
      soloArc(focusId);
      const arcNodes = allNodes.filter((n) => n.story_arc_id === focusId);
      if (arcNodes.length > 0) {
        const maxChapterOfArc = arcNodes.reduce(
          (m, n) => Math.max(m, n.target_chapter || 0),
          0,
        );
        setWindowCenter(Math.max(1, maxChapterOfArc - WINDOW));
        setExpandedId(arcNodes[0].id);
      } else if (maxChQuery.data && maxChQuery.data > 0) {
        setWindowCenter(Math.max(1, maxChQuery.data));
      }
      setHighlightedNodeId(null);
      return;
    }

    if (focusType === "node") {
      // 点击 node 条目 → soloArc(所属弧线) + 窗口对齐到 node 章节 + 展开 + 高亮该 node
      const node = allNodes.find((n) => n.id === focusId);
      if (!node) return;
      soloArc(node.story_arc_id);
      setWindowCenter(node.target_chapter || node.actual_chapter || 1);
      setExpandedId(node.id);
      setHighlightedNodeId(node.id);
      // 滚动到 node 卡片（DOM API 留 useEffect）
      const el = document.querySelector<HTMLElement>(
        `[data-node-id="${node.id}"]`,
      );
      if (el) el.scrollIntoView({ behavior: "smooth", block: "center" });
      const timer = setTimeout(() => setHighlightedNodeId(null), 2000);
      return () => clearTimeout(timer);
    }

    // 兼容无 type 的旧 focus（按 arc 处理，但不 soloArc，避免无谓过滤）
    if (allNodes.length > 0) {
      const arcNodes = allNodes.filter((n) => n.story_arc_id === focusId);
      if (arcNodes.length > 0) {
        const firstNode = arcNodes[0];
        setWindowCenter(
          firstNode.target_chapter || firstNode.actual_chapter || 1,
        );
        setExpandedId(firstNode.id);
      }
    }
  }, [focusId, focusType, focus?.nonce, allNodes, arcs, maxChQuery.data]);

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
    // 4.3.3: 全量回传（§6）— 编辑时把 status 也填进 form，
    // handleUpdateArc 直接传 arcForm 所有字段，等价 PUT。
    setArcForm({
      name: arc.name,
      arc_type: arc.arc_type,
      description: arc.description || "",
      importance: arc.importance,
      status: arc.status,
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
    // 4.3.3: create 走 mutation（onSuccess 失效 storyarcs），删 setSaving/bumpRefresh。
    // input 只传 CreateStoryArcInput 字段（create 不含 status/reactivate_at）。
    try {
      await createArcMutation.mutateAsync({
        name: arcForm.name,
        arc_type: arcForm.arc_type,
        description: arcForm.description,
        importance: arcForm.importance,
      });
      setEditMode(null);
    } catch (err) {
      toastError(t("storyarc.createArcFailed") + ": " + toErrorMessage(err));
      console.error(err);
    }
  }

  async function handleUpdateArc() {
    if (!editMode || editMode.type !== "edit_arc") return;
    // 4.3.3: update 走 mutation（onSuccess 失效 storyarcs），删 setSaving/bumpRefresh。
    // 全量回传 input 所有字段（§6 等价 PUT，openEditArc 已把 status 填进 form）。
    try {
      await updateArcMutation.mutateAsync({
        id: editMode.arc.id,
        input: {
          name: arcForm.name,
          arc_type: arcForm.arc_type,
          description: arcForm.description,
          importance: arcForm.importance,
          status: arcForm.status,
        },
      });
      setEditMode(null);
    } catch (err) {
      toastError(t("storyarc.updateArcFailed") + ": " + toErrorMessage(err));
      console.error(err);
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
    // 4.3.3: 全量回传（§6）— 编辑时把 actual_chapter/status 也填进 form，
    // handleUpdateNode 直接传 nodeForm 所有字段，等价 PUT。
    setNodeForm({
      story_arc_id: node.story_arc_id,
      title: node.title,
      description: node.description || "",
      target_chapter: node.target_chapter,
      actual_chapter: node.actual_chapter,
      status: node.status,
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
    try {
      // 4.3.3: create 走 mutation（onSuccess 失效 arc-nodes），删 setSaving/bumpRefresh。
      // 返回值 created.id 用于 setExpandedId（副作用各异，不放进 mutation）。
      const created = await createNodeMutation.mutateAsync({
        story_arc_id: nodeForm.story_arc_id,
        title: nodeForm.title,
        description: nodeForm.description,
        target_chapter: nodeForm.target_chapter,
      });
      setEditMode(null);
      setExpandedId(created.id);
    } catch (err) {
      toastError(t("storyarc.createNodeFailed") + ": " + toErrorMessage(err));
      console.error(err);
    }
  }

  async function handleUpdateNode() {
    if (!editMode || editMode.type !== "edit_node") return;
    if (!nodeForm.title.trim()) {
      toastError(t("storyarc.pleaseEnterNodeTitle"));
      return;
    }
    // 4.3.3: update 走 mutation（onSuccess 失效 arc-nodes），删 setSaving/bumpRefresh。
    // 全量回传 input 所有字段（§6 等价 PUT，openEditNode 已把 actual_chapter/status 填进 form）。
    const nodeId = editMode.node.id;
    try {
      await updateNodeMutation.mutateAsync({
        id: nodeId,
        input: {
          title: nodeForm.title,
          description: nodeForm.description,
          target_chapter: nodeForm.target_chapter,
          actual_chapter: nodeForm.actual_chapter,
          status: nodeForm.status,
        },
      });
      setEditMode(null);
      setExpandedId(nodeId);
    } catch (err) {
      toastError(t("storyarc.updateNodeFailed") + ": " + toErrorMessage(err));
      console.error(err);
    }
  }

  function handleDeleteNode(nodeId: number) {
    setDeleteTarget({ kind: "node", id: nodeId });
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    // 4.3.2: delete 走 mutation（onSuccess 失效对应 query），删 setDeleting + bumpRefresh。
    try {
      if (deleteTarget.kind === "arc") {
        await deleteArcMutation.mutateAsync(deleteTarget.id);
      } else {
        await deleteNodeMutation.mutateAsync(deleteTarget.id);
      }
      setExpandedId(null);
      setDeleteTarget(null);
    } catch (err) {
      const key =
        deleteTarget.kind === "arc"
          ? "storyarc.deleteArcFailed"
          : "storyarc.deleteNodeFailed";
      toastError(t(key) + ": " + toErrorMessage(err));
      console.error(err);
    }
  }

  async function handleQuickNodeStatus(
    node: storyarc.ArcNode,
    newStatus: string,
  ) {
    // 4.3.3: 走 updateNodeMutation（onSuccess 失效 arc-nodes），删 setSaving/bumpRefresh。
    // 全量回传 input 所有字段（§6 等价 PUT）：其他字段传 node 原值，status 传 newStatus。
    try {
      await updateNodeMutation.mutateAsync({
        id: node.id,
        input: {
          title: node.title,
          description: node.description,
          target_chapter: node.target_chapter,
          actual_chapter: node.actual_chapter,
          status: newStatus,
        },
      });
    } catch (err) {
      toastError(
        t("storyarc.updateNodeStatusFailed") + ": " + toErrorMessage(err),
      );
      console.error(err);
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
              {ARC_TYPES.map((v) => (
                <option key={v} value={v}>
                  {t(`storyarc.${v}`)}
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
                    queryClient.invalidateQueries({
                      queryKey: storyarcKeys.list(novelId),
                    });
                    queryClient.invalidateQueries({
                      queryKey: arcNodeKeys.list(novelId),
                    });
                    queryClient.invalidateQueries({
                      queryKey: maxChapterKeys.detail(novelId),
                    });
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
                            data-node-id={node.id}
                            className={`rounded-lg border bg-card transition-shadow group ${isExpanded ? "border-border shadow-sm" : "border-border hover:border-border hover:shadow-sm"} ${highlightedNodeId === node.id ? "ring-2 ring-primary" : ""}`}
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

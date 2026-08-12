import { useEffect, useMemo, useRef, useState } from "react";
import { Graph } from "@antv/g6";
import {
  ArrowLeft,
  ArrowRight,
  GitBranch,
  LocateFixed,
  RefreshCw,
  X,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useGraphColors } from "@/components/graphColors";
import { useThemeStore } from "@/stores/useThemeStore";
import { arcPalette } from "./arcColors";
import { storyarcKeys, arcNodeKeys, maxChapterKeys } from "@/lib/queryKeys";
import type { storyarc } from "@/lib/wailsjs/go/models";
import { useStoryArcs } from "./useStoryArcs";
import { useArcNodes } from "./useArcNodes";
import { useMaxChapterNumber } from "./useMaxChapterNumber";

interface Props {
  novelId: number;
}

const CH_W = 90;
const LANE_H = 80;
const LEFT_MARGIN = 120;
const NODE_W = 88;
const NODE_H = 30;
const WINDOW = 30;

function nid(id: number) {
  return `an-${id}`;
}
function aid(id: number) {
  return `arc-${id}`;
}
function eid(a: number, b: number) {
  return `e-${a}-${b}`;
}

function centerOnChapter(
  graph: Graph,
  containerW: number,
  containerH: number,
  ch: number,
  wf: number,
  laneCount: number,
) {
  const cx = LEFT_MARGIN + (ch - wf + 0.5) * CH_W;
  const cy = (laneCount * LANE_H) / 2 + 30;
  // translateTo moves canvas origin to (tx, ty) in viewport coordinates.
  // To center canvas point (cx, cy): tx = vw/2 - cx, ty = vh/2 - cy
  graph.translateTo([containerW / 2 - cx, containerH / 2 - cy], false);
}

export default function StoryArcGraph({ novelId }: Props) {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  const C = useGraphColors();
  const { theme } = useThemeStore();
  const PALETTE = arcPalette(theme);
  const containerRef = useRef<HTMLDivElement>(null);
  const graphRef = useRef<Graph | null>(null);

  // 4.3.1: arcs/allNodes/maxChapter 走 query（与 ArcList / ArcListView 共享缓存）。
  // 删原 useApp.GetStoryArcs/GetArcNodes/GetMaxChapterNumber + load() + useState；
  // CRUD 后由 invalidateQueries 触发 refetch（commit 2/3 抽 mutation）。
  // 4a: query 错误 toast 由全局中间件接管（queryErrorToast.ts），此处不再挂 useEffect。
  const arcsQuery = useStoryArcs(novelId);
  const nodesQuery = useArcNodes(novelId);
  const maxChQuery = useMaxChapterNumber(novelId);
  const arcs = arcsQuery.data ?? [];
  const allNodes = nodesQuery.data ?? [];
  const loading = arcsQuery.isLoading || nodesQuery.isLoading;
  // 4.3.1: loadFailed 只看 arcs（arcs 失败整图不可用）。
  // nodes 失败时图仍渲染（空节点），nodes 错误 toast 由中间件弹，不阻塞图渲染。
  const loadFailed = arcsQuery.isError;

  const [selectedNode, setSelectedNode] = useState<storyarc.ArcNode | null>(
    null,
  );
  const [selectedArc, setSelectedArc] = useState<storyarc.StoryArc | null>(
    null,
  );
  const [expanded, setExpanded] = useState(false);
  const [windowCenter, setWindowCenter] = useState(0);
  const [edgeCounts, setEdgeCounts] = useState({ left: 0, right: 0 });

  // 4.3.1: maxChapter 就绪后初始化 windowCenter（替代原 load() 里的 setWindowCenter）。
  // 只在 maxChapter 变化时设，不覆盖用户后续 shift 操作。
  useEffect(() => {
    const max = maxChQuery.data ?? 0;
    if (max > 0) setWindowCenter(Math.max(1, max));
  }, [maxChQuery.data]);

  const windowFrom = useMemo(
    () => Math.max(1, windowCenter - WINDOW),
    [windowCenter],
  );
  const windowTo = useMemo(() => windowCenter + WINDOW, [windowCenter]);
  const windowFromRef = useRef(windowFrom);
  const windowToRef = useRef(windowTo);
  useEffect(() => {
    windowFromRef.current = windowFrom;
    windowToRef.current = windowTo;
  }, [windowFrom, windowTo]);
  const autoExpandRef = useRef(false);

  const totalChapters =
    allNodes.length > 0
      ? Math.max(...allNodes.map((n) => n.target_chapter))
      : 1;
  const totalChaptersRef = useRef(totalChapters);
  const allNodesRef = useRef(allNodes);
  useEffect(() => {
    totalChaptersRef.current = totalChapters;
    allNodesRef.current = allNodes;
  }, [totalChapters, allNodes]);

  const nodesByArc = useMemo(() => {
    const map = new Map<number, storyarc.ArcNode[]>();
    for (const n of allNodes) {
      if (!map.has(n.story_arc_id)) map.set(n.story_arc_id, []);
      map.get(n.story_arc_id)!.push(n);
    }
    for (const [, ns] of map) {
      ns.sort((a, b) => a.target_chapter - b.target_chapter || a.id - b.id);
    }
    return map;
  }, [allNodes]);

  const graphData = useMemo(() => {
    if (arcs.length === 0) return { nodes: [], edges: [] };

    const visibleNodes = allNodes.filter(
      (n) => n.target_chapter >= windowFrom && n.target_chapter <= windowTo,
    );

    const gNodes: any[] = [];
    const gEdges: any[] = [];

    // Arc lane labels
    arcs.forEach((arc, i) => {
      const color = PALETTE[i % PALETTE.length];
      const statusSuffix = (() => {
        switch (arc.status) {
          case "paused":
            return " ⏸";
          case "completed":
            return " ✓";
          case "abandoned":
            return " ✗";
          default:
            return "";
        }
      })();
      const dim = arc.status === "abandoned";
      gNodes.push({
        id: aid(arc.id),
        type: "rect",
        style: {
          size: [LEFT_MARGIN - 12, NODE_H],
          x: LEFT_MARGIN / 2,
          y: i * LANE_H + LANE_H / 2,
          fill: dim ? C.dimFill : color.fill,
          stroke: dim ? C.dimStroke : color.stroke,
          radius: 8,
          lineWidth: 2,
          labelText: arc.name + statusSuffix,
          labelFontSize: 12,
          labelFontWeight: 600,
          labelFill: dim ? C.dimText : color.text,
          labelPlacement: "center" as const,
          cursor: "pointer",
        },
        data: { arc },
      });
    });

    // Chapter ruler marks (every 5 chapters)
    for (let ch = Math.floor(windowFrom / 5) * 5; ch <= windowTo; ch += 5) {
      if (ch < 1) continue;
      const x = LEFT_MARGIN + (ch - windowFrom) * CH_W;
      gNodes.push({
        id: `ch-${ch}`,
        type: "rect",
        style: {
          size: [34, 22],
          x,
          y: 10,
          fill: C.dimFill,
          stroke: C.dimStroke,
          radius: 4,
          lineWidth: 1,
          labelText: String(ch),
          labelFontSize: 12,
          labelFontWeight: 600,
          labelFill: C.dimText,
          labelPlacement: "center" as const,
        },
        data: {},
      });
    }

    // Arc nodes
    for (const n of visibleNodes) {
      const arcIdx = arcs.findIndex((a) => a.id === n.story_arc_id);
      if (arcIdx < 0) continue;
      const color = PALETTE[arcIdx % PALETTE.length];
      const x = LEFT_MARGIN + (n.target_chapter - windowFrom + 0.5) * CH_W;
      const y = arcIdx * LANE_H + LANE_H / 2;

      let fill = color.fill;
      let strokeColor = color.stroke;
      let textColor = color.text;
      let opacity = 1;
      let dash: number[] | undefined;

      if (n.status === "pending") {
        fill = C.card;
      } else if (n.status === "abandoned") {
        fill = C.bg;
        strokeColor = C.dimStroke;
        textColor = C.dimText;
        opacity = 0.55;
        dash = [3, 3];
      }

      const label = n.title.length > 5 ? n.title.slice(0, 5) + "…" : n.title;

      gNodes.push({
        id: nid(n.id),
        type: "rect",
        style: {
          size: [NODE_W, NODE_H],
          x,
          y,
          fill,
          stroke: strokeColor,
          radius: NODE_H / 2,
          lineWidth: 2,
          lineDash: dash,
          opacity,
          labelText: label,
          labelFontSize: 11,
          labelFontWeight: 500,
          labelFill: textColor,
          labelPlacement: "center" as const,
          cursor: "pointer",
        },
        data: { arcNode: n, arcIdx },
      });
    }

    // Ghost anchors at window edges for off-screen connections
    const rightEdgeX = LEFT_MARGIN + (windowTo - windowFrom + 1) * CH_W;
    arcs.forEach((arc, i) => {
      const y = i * LANE_H + LANE_H / 2;
      for (const side of ["l", "r"]) {
        gNodes.push({
          id: `ghost-${side}-${arc.id}`,
          type: "rect",
          style: {
            size: [1, 1],
            x: side === "l" ? LEFT_MARGIN : rightEdgeX,
            y,
            fill: "transparent",
            stroke: "transparent",
            opacity: 0,
            cursor: "default",
          },
          data: {},
        });
      }
    });

    function edgeStyle(src: storyarc.ArcNode, tgt: storyarc.ArcNode) {
      const color =
        PALETTE[
          arcs.findIndex((a) => a.id === src.story_arc_id) % PALETTE.length
        ];
      let stroke = color.edge;
      let lineDash: number[] | undefined;
      let opacity = 1;
      let arrow = false;
      if (src.status === "abandoned" || tgt.status === "abandoned") {
        stroke = C.dimText;
        lineDash = [4, 4];
        opacity = 0.5;
      } else if (src.status === "pending" && tgt.status === "pending") {
        lineDash = [6, 4];
        opacity = 0.7;
      } else if (src.status === "completed" && tgt.status === "pending") {
        arrow = true;
      }
      return { stroke, lineDash, opacity, arrow };
    }

    // Edges
    for (const arc of arcs) {
      const ns = nodesByArc.get(arc.id);
      if (!ns || ns.length < 2) continue;

      for (let i = 0; i < ns.length - 1; i++) {
        const src = ns[i];
        const tgt = ns[i + 1];
        const srcCh = src.target_chapter;
        const tgtCh = tgt.target_chapter;

        // Both outside window on same side — skip
        if (srcCh > windowTo && tgtCh > windowTo) continue;
        if (srcCh < windowFrom && tgtCh < windowFrom) continue;

        const srcVis = srcCh >= windowFrom && srcCh <= windowTo;
        const tgtVis = tgtCh >= windowFrom && tgtCh <= windowTo;

        const sourceId = srcVis ? nid(src.id) : `ghost-l-${arc.id}`;
        const targetId = tgtVis ? nid(tgt.id) : `ghost-r-${arc.id}`;
        const crossing = !srcVis && !tgtVis;

        const style = edgeStyle(src, tgt);

        gEdges.push({
          id: eid(src.id, tgt.id),
          source: sourceId,
          target: targetId,
          type: "line",
          style: {
            stroke: style.stroke,
            lineWidth: 2,
            lineDash: crossing ? [6, 4] : style.lineDash,
            opacity: crossing ? 0.5 : style.opacity,
            endArrow: crossing ? false : style.arrow,
            endArrowSize: style.arrow && !crossing ? 8 : 0,
          },
        });
      }
    }

    return { nodes: gNodes, edges: gEdges };
  }, [arcs, allNodes, windowFrom, windowTo, nodesByArc, C, PALETTE]);

  // Create G6 graph once on mount
  useEffect(() => {
    const container = containerRef.current;
    if (!container || loading || arcs.length === 0) return;

    let graph: Graph | null = null;
    try {
      graph = new Graph({
        container,
        data: graphData,
        background: C.bg,
        animation: false,
        node: {
          type: "rect",
          style: {
            radius: NODE_H / 2,
            lineWidth: 2,
            labelPlacement: "center" as const,
            labelOffsetY: 0,
          },
        },
        edge: {
          type: "line",
        },
        behaviors: [
          "drag-canvas",
          "zoom-canvas",
          "optimize-viewport-transform",
        ],
      });
      graphRef.current = graph;
      graph
        .render()
        .then(() => {
          centerOnChapter(
            graph!,
            container.clientWidth,
            container.clientHeight,
            windowCenter,
            windowFrom,
            arcs.length,
          );
        })
        .catch((err: unknown) => {
          console.error("Graph render failed:", err);
        });
    } catch (err) {
      console.error("Graph init/render failed:", err);
      if (graph) {
        try {
          graph.destroy();
        } catch {
          /* ignore */
        }
        if (graphRef.current === graph) graphRef.current = null;
      }
      return;
    }

    graph.on("node:click", (event: any) => {
      const rawId = event.target?.id || "";
      const arcMatch = arcs.find((a) => rawId === aid(a.id));
      if (arcMatch) {
        setSelectedArc((prev) => (prev?.id === arcMatch.id ? null : arcMatch));
        setSelectedNode(null);
        return;
      }
      if (rawId.startsWith("ch-")) return;
      const nodeMatch = allNodes.find((n) => rawId.startsWith(nid(n.id)));
      if (nodeMatch) {
        setSelectedNode((prev) =>
          prev?.id === nodeMatch.id ? null : nodeMatch,
        );
        setSelectedArc(null);
        setExpanded(false);
      }
    });

    // Auto-expand window on drag near edges
    graph.on("canvas:dragend", () => {
      const wf = windowFromRef.current;
      const wt = windowToRef.current;
      const tc = totalChaptersRef.current;
      const vp = graph!.getCanvasByViewport([
        container.clientWidth / 2,
        container.clientHeight / 2,
      ]);
      if (!vp) return;
      const ch = Math.round((vp[0] - LEFT_MARGIN) / CH_W) + wf;
      const margin = Math.max(3, Math.floor(WINDOW / 6));
      if (ch < wf + margin) {
        autoExpandRef.current = true;
        setWindowCenter((prev) =>
          Math.max(WINDOW + 1, prev - Math.floor(WINDOW / 2)),
        );
      } else if (ch > wt - margin) {
        autoExpandRef.current = true;
        setWindowCenter((prev) =>
          Math.min(tc - WINDOW, prev + Math.floor(WINDOW / 2)),
        );
      }
    });

    // Update edge counts on zoom / after drag
    const updateEdgeCounts = () => {
      const zoom = graph!.getZoom();
      const pos = graph!.getPosition();
      const cw = container.clientWidth;
      const wf = windowFromRef.current;
      const left = -pos[0] / zoom;
      const right = left + cw / zoom;
      const visFrom = Math.floor((left - LEFT_MARGIN) / CH_W) + wf;
      const visTo = Math.ceil((right - LEFT_MARGIN) / CH_W) + wf;
      const nodes = allNodesRef.current;
      setEdgeCounts({
        left: nodes.filter((n) => n.target_chapter < visFrom).length,
        right: nodes.filter((n) => n.target_chapter > visTo).length,
      });
    };
    graph.on("canvas:zoom", updateEdgeCounts);
    graph.on("canvas:dragend", updateEdgeCounts);
    setTimeout(updateEdgeCounts, 200);

    const ro = new ResizeObserver(() => graph!.resize());
    ro.observe(container);

    return () => {
      ro.disconnect();
      graph!.destroy();
      graphRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- graphData/windowCenter/windowFrom are handled by the second effect that updates graph data in place
  }, [arcs, allNodes, C.bg, C.edge, loading]);

  // Update graph data when window shifts
  useEffect(() => {
    const graph = graphRef.current;
    if (!graph || arcs.length === 0) return;
    let cancelled = false;
    graph.setData(graphData);
    graph
      .draw()
      .then(() => {
        if (cancelled) return;
        if (autoExpandRef.current) {
          autoExpandRef.current = false;
          return;
        }
        const cw = containerRef.current?.clientWidth ?? 800;
        const ch = containerRef.current?.clientHeight ?? 600;
        centerOnChapter(graph, cw, ch, windowCenter, windowFrom, arcs.length);
      })
      .catch((err: unknown) => {
        if (!cancelled) console.error("Graph draw failed:", err);
      });
    return () => {
      cancelled = true;
    };
  }, [graphData, arcs.length, windowCenter, windowFrom]);

  const canShiftLeft = windowCenter > WINDOW + 1;
  const canShiftRight = windowTo < totalChapters;

  function shift(delta: number) {
    setWindowCenter((prev) =>
      Math.max(WINDOW + 1, Math.min(totalChapters - WINDOW, prev + delta)),
    );
  }

  return (
    <main className="relative flex-1 min-w-0 overflow-hidden bg-background">
      {/* Toolbar */}
      <div className="absolute left-5 right-5 top-4 z-10 flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2 text-sm font-semibold text-foreground">
            <GitBranch className="h-4 w-4 text-tag-purple-foreground" />
            {t("storyarc.storyArcsTitle")}
          </div>
          <div className="mt-1 text-xs text-muted-foreground">
            {t("storyarc.arcCount", { count: arcs.length })} ·{" "}
            {t("storyarc.nodeCountTotal", { count: allNodes.length })} ·{" "}
            {t("sidebar.chapterRange", { start: windowFrom, end: windowTo })}（
            {t("storyarc.totalChapters", { count: totalChapters })}）
          </div>
        </div>
        <div className="flex items-center gap-1.5 rounded-md border border-border/80 bg-card/82 p-1 shadow-sm backdrop-blur">
          <button
            type="button"
            onClick={() => shift(-20)}
            disabled={!canShiftLeft}
            className="flex h-7 w-7 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground disabled:opacity-30 disabled:cursor-default select-none"
            title={t("storyarc.forward20")}
          >
            <ArrowLeft className="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            onClick={() => shift(20)}
            disabled={!canShiftRight}
            className="flex h-7 w-7 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground disabled:opacity-30 disabled:cursor-default select-none"
            title={t("storyarc.backward20")}
          >
            <ArrowRight className="h-3.5 w-3.5" />
          </button>
          <div className="w-px h-4 bg-muted" />
          <button
            type="button"
            onClick={() =>
              graphRef.current?.fitView(
                { when: "always" },
                { duration: 360, easing: "ease-in-out" },
              )
            }
            className="flex h-7 w-7 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
            title={t("storyarc.fitView")}
          >
            <LocateFixed className="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
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
            className="flex h-7 w-7 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
            title={t("storyarc.refresh")}
          >
            <RefreshCw className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>

      {/* Legend */}
      <div className="absolute right-0 bottom-0 z-10 rounded-md border border-border bg-card/90 px-3 py-2.5 text-xs text-muted-foreground shadow-sm backdrop-blur space-y-2.5">
        <div>
          <span className="text-muted-foreground text-[10px] uppercase tracking-wider">
            {t("storyarc.legendNodes")}
          </span>
          <div className="flex items-center gap-3 mt-1.5">
            <span className="inline-flex items-center gap-1.5">
              <span className="inline-block h-3.5 w-8 rounded-full bg-tag-blue-foreground border border-tool-blue-border" />
              {t("storyarc.completed")}
            </span>
            <span className="inline-flex items-center gap-1.5">
              <span className="inline-block h-3.5 w-8 rounded-full border-2 border-tool-blue-border bg-card" />
              {t("storyarc.inProgress")}
            </span>
            <span className="inline-flex items-center gap-1.5">
              <span className="inline-block h-3.5 w-8 rounded-full bg-muted border border-dashed border-border" />
              {t("storyarc.abandoned")}
            </span>
          </div>
        </div>
        <div>
          <span className="text-muted-foreground text-[10px] uppercase tracking-wider">
            {t("storyarc.legendEdges")}
          </span>
          <div className="flex items-center gap-3 mt-1.5">
            <span className="inline-flex items-center gap-1.5">
              <span className="inline-block h-0.5 w-7 bg-tag-blue-foreground rounded" />
              {t("storyarc.occurred")}
            </span>
            <span className="inline-flex items-center gap-1.5">
              <span className="inline-block h-0 w-7 border-t-2 border-dashed border-tool-blue-border" />
              {t("storyarc.notOccurred")}
            </span>
            <span className="inline-flex items-center gap-1.5">
              <span className="inline-block h-0 w-7 border-t-2 border-dashed border-border" />
              {t("storyarc.broken")}
            </span>
          </div>
        </div>
      </div>

      {/* Detail panel: selected node */}
      {selectedNode &&
        (() => {
          const desc = selectedNode.description?.trim() || "";
          const longDesc = desc.length > 100;
          const ch =
            selectedNode.actual_chapter > 0
              ? t("storyarc.actualChapter", { n: selectedNode.actual_chapter })
              : t("storyarc.targetChapter2", {
                  n: selectedNode.target_chapter,
                });
          const arc = arcs.find((a) => a.id === selectedNode.story_arc_id);
          return (
            <div className="absolute left-5 bottom-5 z-10 w-64 rounded-lg border border-border bg-card/94 p-4 shadow-lg backdrop-blur text-sm">
              <div className="flex items-start justify-between gap-2 mb-2">
                <h3 className="font-semibold text-foreground">
                  {selectedNode.title}
                </h3>
                <button
                  onClick={() => {
                    setSelectedNode(null);
                    setExpanded(false);
                  }}
                  className="shrink-0 rounded p-0.5 text-muted-foreground hover:text-muted-foreground hover:bg-secondary transition-colors"
                >
                  <X className="h-3.5 w-3.5" />
                </button>
              </div>
              {arc && (
                <span className="inline-block rounded bg-tag-purple px-2 py-0.5 text-xs text-tag-purple-foreground mb-2">
                  {arc.name}
                </span>
              )}
              <div className="text-xs text-muted-foreground mb-2">
                <span
                  className={
                    selectedNode.status === "completed"
                      ? "text-tag-green-foreground"
                      : selectedNode.status === "abandoned"
                        ? "text-muted-foreground line-through"
                        : "text-tag-blue-foreground"
                  }
                >
                  {selectedNode.status === "completed"
                    ? t("storyarc.completed")
                    : selectedNode.status === "abandoned"
                      ? t("storyarc.abandoned")
                      : t("storyarc.inProgress")}
                </span>
                <span className="mx-1.5">·</span>
                <span>{ch}</span>
              </div>
              {desc && (
                <div>
                  <p className="text-xs text-muted-foreground leading-relaxed">
                    {longDesc && !expanded ? desc.slice(0, 100) + "…" : desc}
                  </p>
                  {longDesc && (
                    <button
                      onClick={() => setExpanded(!expanded)}
                      className="text-xs text-tag-blue-foreground hover:text-tag-blue-foreground mt-0.5"
                    >
                      {expanded
                        ? t("character.collapse")
                        : t("character.expand")}
                    </button>
                  )}
                </div>
              )}
              {!desc && (
                <p className="text-xs text-muted-foreground">
                  {t("storyarc.noDetailDescription")}
                </p>
              )}
            </div>
          );
        })()}

      {/* Detail panel: selected arc */}
      {selectedArc && !selectedNode && (
        <div className="absolute left-5 bottom-5 z-10 w-64 rounded-lg border border-border bg-card/94 p-4 shadow-lg backdrop-blur text-sm">
          <div className="flex items-start justify-between gap-2 mb-2">
            <h3 className="font-semibold text-foreground">
              {selectedArc.name}
            </h3>
            <button
              onClick={() => setSelectedArc(null)}
              className="shrink-0 rounded p-0.5 text-muted-foreground hover:text-muted-foreground hover:bg-secondary transition-colors"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          </div>
          <div className="flex items-center gap-2 mb-2">
            <span className="inline-block rounded bg-secondary px-2 py-0.5 text-xs text-muted-foreground">
              {selectedArc.arc_type}
            </span>
            <span
              className={`
              inline-block rounded px-2 py-0.5 text-xs
              ${selectedArc.status === "active" ? "bg-tag-green text-tag-green-foreground" : ""}
              ${selectedArc.status === "paused" ? "bg-tag-amber text-tag-amber-foreground" : ""}
              ${selectedArc.status === "completed" ? "bg-tag-blue text-tag-blue-foreground" : ""}
              ${selectedArc.status === "abandoned" ? "bg-secondary text-muted-foreground" : ""}
            `}
            >
              {selectedArc.status === "active"
                ? t("storyarc.active")
                : selectedArc.status === "paused"
                  ? t("storyarc.paused")
                  : selectedArc.status === "completed"
                    ? t("storyarc.completed")
                    : selectedArc.status === "abandoned"
                      ? t("storyarc.abandoned")
                      : selectedArc.status}
            </span>
            <span className="text-xs text-muted-foreground">
              {"★".repeat(selectedArc.importance)}
            </span>
          </div>
          {selectedArc.description && (
            <p className="text-xs text-muted-foreground leading-relaxed">
              {selectedArc.description}
            </p>
          )}
          {selectedArc.status === "paused" && selectedArc.reactivate_at && (
            <div className="mt-2 pt-2 border-t border-border">
              <p className="text-xs text-muted-foreground mb-0.5">
                {t("storyarc.resumeCondition")}
              </p>
              <p className="text-xs text-muted-foreground">
                {selectedArc.reactivate_at}
              </p>
            </div>
          )}
        </div>
      )}

      {/* Edge indicators */}
      {edgeCounts.left > 0 && (
        <button
          onClick={() => shift(-WINDOW)}
          className="absolute left-5 top-1/2 z-10 -translate-y-1/2 rounded-full border border-border bg-card/88 px-2 py-1.5 text-[10px] text-muted-foreground shadow-sm backdrop-blur hover:bg-card hover:text-foreground transition-colors"
        >
          ← {t("storyarc.nodeCount", { count: edgeCounts.left })}
        </button>
      )}
      {edgeCounts.right > 0 && (
        <button
          onClick={() => shift(WINDOW)}
          className="absolute right-5 top-1/2 z-10 -translate-y-1/2 rounded-full border border-border bg-card/88 px-2 py-1.5 text-[10px] text-muted-foreground shadow-sm backdrop-blur hover:bg-card hover:text-foreground transition-colors"
        >
          {t("storyarc.nodeCount", { count: edgeCounts.right })} →
        </button>
      )}

      {/* G6 container */}
      {loading ? (
        <div className="relative z-10 flex h-full items-center justify-center text-sm text-muted-foreground">
          {t("storyarc.loading")}
        </div>
      ) : loadFailed ? (
        <div className="relative z-10 flex h-full items-center justify-center">
          <p className="text-xs text-destructive py-4">
            {t("storyarc.arcsLoadFailed")}
          </p>
        </div>
      ) : arcs.length === 0 ? (
        <div className="relative z-10 flex h-full items-center justify-center">
          <div className="text-center">
            <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-tag-purple text-tag-purple-foreground shadow-sm">
              <GitBranch className="h-6 w-6" />
            </div>
            <div className="mt-3 text-sm font-medium text-foreground">
              {t("storyarc.noNarrativeArcs2")}
            </div>
          </div>
        </div>
      ) : (
        <div ref={containerRef} className="relative z-[1] h-full w-full" />
      )}
    </main>
  );
}

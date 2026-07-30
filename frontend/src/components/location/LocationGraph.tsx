import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Graph, treeToGraphData } from "@antv/g6";
import { LocateFixed, Map as MapIcon, RefreshCw, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useApp } from "@/hooks/useApp";
import { useGraphColors } from "@/components/graphColors";
import type { location } from "@/hooks/useApp";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";

interface Props {
  novelId: number;
  focusId?: number;
}

const NODE_COLOR = { fill: "#dbeafe", stroke: "#3b82f6", text: "#1d4ed8" };

function nodeId(id: number) {
  return `location-${id}`;
}

function safeJson<T>(json: string, fallback: T): T {
  if (json == null) return fallback;
  try {
    return JSON.parse(json);
  } catch {
    return fallback;
  }
}

function buildTreeData(locs: location.Location[]) {
  const map = new Map<number, any>();
  const roots: any[] = [];
  for (const loc of locs)
    map.set(loc.id, {
      id: nodeId(loc.id),
      data: { name: loc.name, type: loc.location_type },
      children: [],
    });
  for (const loc of locs) {
    const node = map.get(loc.id)!;
    if (loc.parent_location_id && map.has(loc.parent_location_id)) {
      map.get(loc.parent_location_id).children.push(node);
    } else {
      roots.push(node);
    }
  }
  if (roots.length === 1) return roots[0];
  return { id: "__root__", data: { name: "", type: "" }, children: roots };
}

export default function LocationGraph({ novelId, focusId }: Props) {
  const app = useApp();
  const { t } = useTranslation();
  const C = useGraphColors();
  const containerRef = useRef<HTMLDivElement>(null);
  const graphRef = useRef<Graph | null>(null);

  const [locations, setLocations] = useState<location.Location[]>([]);
  const [relations, setRelations] = useState<location.LocationRelation[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadFailed, setLoadFailed] = useState(false);
  const [selectedLocation, setSelectedLocation] =
    useState<location.Location | null>(null);
  const [expanded, setExpanded] = useState(false);

  const load = useCallback(async () => {
    if (!novelId) {
      setLocations([]);
      setRelations([]);
      return;
    }
    setLoading(true);
    setLoadFailed(false);
    try {
      const [locList, relList] = await Promise.all([
        app.GetLocations(novelId),
        app.GetLocationRelations(novelId),
      ]);
      setLocations(locList ?? []);
      setRelations(relList ?? []);
    } catch (err) {
      setLoadFailed(true);
      toastError(
        t("location.loadFailed") +
          ": " +
          toErrorMessage(err),
      );
      console.error(err);
    } finally {
      setLoading(false);
    }
  }, [app, novelId, t]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    if (focusId && focusId > 0 && locations.length > 0) {
      const loc = locations.find((l) => l.id === focusId);
      if (loc) setSelectedLocation(loc);
    }
  }, [focusId, locations]);

  const graphData = useMemo(() => {
    const locIds = new Set(locations.map((l) => l.id));
    const treeData = buildTreeData(locations);
    let baseGraph: any;
    try {
      baseGraph = treeToGraphData(treeData);
    } catch (err) {
      console.error("treeToGraphData failed:", err);
      return { nodes: [], edges: [] };
    }

    const nodes = (baseGraph.nodes ?? []).map((n: any) => {
      const loc = locations.find((l) => nodeId(l.id) === n.id);
      if (!loc) return n;
      const isRoot = !loc.parent_location_id;
      return {
        ...n,
        data: { location: loc },
        style: {
          size: [90, 34],
          fill: NODE_COLOR.fill,
          stroke: NODE_COLOR.stroke,
          labelText: loc.name,
          labelFontSize: isRoot ? 14 : 13,
          labelFontWeight: isRoot ? 700 : 500,
          labelFill: NODE_COLOR.text,
          labelPlacement: "center" as const,
          labelOffsetY: 0,
        },
      };
    });

    const containEdges = (baseGraph.edges ?? [])
      .filter((e: any) => e.source !== "__root__" && e.target !== "__root__")
      .map((e: any) => ({
        ...e,
        type: "polyline",
        style: {
          stroke: C.edge,
          lineWidth: 2,
          endArrow: true,
          endArrowSize: 10,
        },
      }));

    const containPairs = new Set(
      locations
        .filter((l) => l.parent_location_id)
        .map((l) => `${l.parent_location_id}-${l.id}`),
    );
    const spaceEdges = relations
      .filter((r) => {
        if (!locIds.has(r.location_a_id) || !locIds.has(r.location_b_id))
          return false;
        if (r.location_a_id === r.location_b_id) return false;
        if (containPairs.has(`${r.location_a_id}-${r.location_b_id}`))
          return false;
        if (containPairs.has(`${r.location_b_id}-${r.location_a_id}`))
          return false;
        return true;
      })
      .map((r, i) => ({
        id: `space-${r.id || i}`,
        source: nodeId(r.location_a_id),
        target: nodeId(r.location_b_id),
        type: "line",
        data: { relation: r },
        style: {
          stroke: C.edgeDim,
          lineWidth: 1,
          lineDash: [4, 4],
        },
      }));

    return { nodes, edges: [...containEdges, ...spaceEdges] };
  }, [locations, relations, C]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container || loading || locations.length === 0) return;

    let graph: Graph | null = null;
    try {
      graph = new Graph({
        container,
        data: graphData,
        autoFit: "view",
        background: C.bg,
        animation: false,
        node: {
          type: "rect",
          style: {
            radius: 17,
            lineWidth: 2,
            cursor: "pointer",
            labelPlacement: "center" as const,
            labelOffsetY: 0,
          },
        },
        edge: {
          type: "polyline",
          style: {
            stroke: C.edge,
            lineWidth: 2,
          },
        },
        layout: {
          type: "dagre",
          rankdir: "LR",
          nodesep: 50,
          ranksep: 120,
        },
        behaviors: [
          "drag-canvas",
          "zoom-canvas",
          "drag-element",
          "optimize-viewport-transform",
        ],
      });
      graphRef.current = graph;
      graph.render().catch((err: unknown) => {
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
      const loc = locations.find((l) => rawId.startsWith(nodeId(l.id)));
      if (loc) {
        setSelectedLocation((prev) => (prev?.id === loc.id ? null : loc));
        setExpanded(false);
      }
    });

    const ro = new ResizeObserver(() => graph!.resize());
    ro.observe(container);

    return () => {
      ro.disconnect();
      graph!.destroy();
      if (graphRef.current === graph) graphRef.current = null;
    };
  }, [locations, C.bg, C.edge, graphData, loading]);

  const containCount = locations.filter((l) => l.parent_location_id).length;

  return (
    <main className="relative flex-1 min-w-0 overflow-hidden bg-background">
      <div className="absolute left-5 right-5 top-4 z-10 flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2 text-sm font-semibold text-foreground">
            <MapIcon className="h-4 w-4 text-tag-teal-foreground" />
            {t("location.relationGraphTitle")}
          </div>
          <div className="mt-1 text-xs text-muted-foreground">
            {t("location.locationCount", { count: locations.length })} ·{" "}
            {t("location.containRelationCount", { count: containCount })} ·{" "}
            {t("location.spatialRelationCount", { count: relations.length })}
          </div>
        </div>
        <div className="flex items-center gap-1.5 rounded-md border border-border/80 bg-card/82 p-1 shadow-sm backdrop-blur">
          <button
            type="button"
            onClick={() =>
              graphRef.current?.fitView(
                { when: "always" },
                { duration: 360, easing: "ease-in-out" },
              )
            }
            className="flex h-7 w-7 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
            title={t("location.fitView")}
          >
            <LocateFixed className="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            onClick={load}
            className="flex h-7 w-7 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
            title={t("location.refresh")}
          >
            <RefreshCw className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>

      <div className="absolute right-5 bottom-5 z-10 flex items-center gap-3 rounded-md border border-border bg-card/84 px-3 py-2 text-[11px] text-muted-foreground shadow-sm backdrop-blur">
        <span className="inline-flex items-center gap-1.5">
          <span className="h-0.5 w-7 rounded bg-tag-blue-foreground" />
          {t("location.contain")}
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="h-0 w-7 border-t border-dashed border-border" />
          {t("location.spatial")}
        </span>
      </div>

      {selectedLocation &&
        (() => {
          const detail = safeJson<Record<string, any>>(
            selectedLocation.detail_json,
            {},
          );
          const tags: string[] = safeJson<string[]>(selectedLocation.tags, []);
          const desc = selectedLocation.description?.trim() || "";
          const detailKeys = Object.keys(detail).filter((k) => k.trim());
          const hasContent =
            desc ||
            detailKeys.length > 0 ||
            tags.length > 0 ||
            selectedLocation.location_type;
          const longDesc = desc.length > 100;

          return (
            <div className="absolute left-5 bottom-5 z-10 w-64 rounded-lg border border-border bg-card/94 p-4 shadow-lg backdrop-blur text-sm">
              <div className="flex items-start justify-between gap-2 mb-2">
                <h3 className="font-semibold text-foreground">
                  {selectedLocation.name}
                </h3>
                <button
                  onClick={() => {
                    setSelectedLocation(null);
                    setExpanded(false);
                  }}
                  className="shrink-0 rounded p-0.5 text-muted-foreground hover:text-muted-foreground hover:bg-secondary transition-colors"
                >
                  <X className="h-3.5 w-3.5" />
                </button>
              </div>

              {selectedLocation.location_type && (
                <span className="inline-block rounded bg-secondary px-2 py-0.5 text-xs text-muted-foreground mb-2">
                  {selectedLocation.location_type}
                </span>
              )}

              {!hasContent ? (
                <p className="text-xs text-muted-foreground">
                  {t("location.noDetailInfo")}
                </p>
              ) : (
                <div className="space-y-3">
                  {desc && (
                    <div>
                      <p className="text-xs text-muted-foreground leading-relaxed">
                        {longDesc && !expanded
                          ? desc.slice(0, 100) + "…"
                          : desc}
                      </p>
                      {longDesc && (
                        <button
                          onClick={() => setExpanded(!expanded)}
                          className="text-xs text-tag-blue-foreground hover:text-tag-blue-foreground mt-0.5"
                        >
                          {expanded
                            ? t("location.collapse")
                            : t("location.expand")}
                        </button>
                      )}
                    </div>
                  )}
                  {detailKeys.length > 0 && (
                    <div className="space-y-1">
                      {detailKeys.map((k) => {
                        const v = detail[k];
                        const display =
                          typeof v === "string"
                            ? v
                            : Array.isArray(v)
                              ? v.join("、")
                              : String(v);
                        return (
                          <div key={k} className="flex gap-2 text-xs">
                            <span className="text-muted-foreground shrink-0">
                              {k}
                            </span>
                            <span className="text-muted-foreground">
                              {display}
                            </span>
                          </div>
                        );
                      })}
                    </div>
                  )}
                  {tags.length > 0 && (
                    <div className="flex flex-wrap gap-1">
                      {tags.map((t, i) => (
                        <span
                          key={i}
                          className="inline-block rounded bg-tag-blue px-2 py-0.5 text-xs text-tag-blue-foreground"
                        >
                          {t}
                        </span>
                      ))}
                    </div>
                  )}
                </div>
              )}
            </div>
          );
        })()}

      {loading ? (
        <div className="relative z-10 flex h-full items-center justify-center text-sm text-muted-foreground">
          {t("location.loading")}
        </div>
      ) : loadFailed ? (
        <div className="relative z-10 flex h-full items-center justify-center">
          <p className="text-xs text-destructive py-4">
            {t("location.loadFailed")}
          </p>
        </div>
      ) : locations.length === 0 ? (
        <div className="relative z-10 flex h-full items-center justify-center">
          <div className="text-center">
            <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-tag-teal text-tag-teal-foreground shadow-sm">
              <MapIcon className="h-6 w-6" />
            </div>
            <div className="mt-3 text-sm font-medium text-foreground">
              {t("location.noLocations")}
            </div>
          </div>
        </div>
      ) : (
        <div ref={containerRef} className="relative z-[1] h-full w-full" />
      )}
    </main>
  );
}

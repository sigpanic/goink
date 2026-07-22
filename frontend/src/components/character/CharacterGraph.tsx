import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Graph, treeToGraphData } from "@antv/g6";
import { LocateFixed, RefreshCw, UsersRound, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useApp } from "@/hooks/useApp";
import { useGraphColors } from "@/components/graphColors";
import type { character } from "@/hooks/useApp";

interface Props {
  novelId: number;
  focusId?: number;
}

function nodeId(id: number) {
  return `character-${id}`;
}

function safeJson<T>(json: string, fallback: T): T {
  if (json == null) return fallback;
  try {
    return JSON.parse(json);
  } catch {
    return fallback;
  }
}

function buildCharacterTree(
  characters: character.Character[],
  relations: character.CharacterRelation[],
) {
  const charMap = new Map(characters.map((c) => [c.id, c]));
  const charIds = new Set(characters.map((c) => c.id));
  const adj = new Map<number, number[]>();
  for (const c of characters) adj.set(c.id, []);
  const validRelations = relations.filter(
    (r) =>
      charIds.has(r.source_character_id) && charIds.has(r.target_character_id),
  );
  for (const r of validRelations) {
    adj.get(r.source_character_id)?.push(r.target_character_id);
    adj.get(r.target_character_id)?.push(r.source_character_id);
  }

  let rootId = characters[0]?.id;
  let maxDeg = -1;
  for (const [id, neighbors] of adj) {
    if (neighbors.length > maxDeg) {
      maxDeg = neighbors.length;
      rootId = id;
    }
  }
  if (!rootId) return { treeData: null, nonTreeEdges: [] };

  const visited = new Set<number>([rootId]);
  const treeEdgeSet = new Set<string>();
  const childrenMap = new Map<number, any[]>();
  for (const c of characters) childrenMap.set(c.id, []);

  const queue = [rootId];
  while (queue.length > 0) {
    const cur = queue.shift()!;
    for (const nb of adj.get(cur) || []) {
      if (!visited.has(nb)) {
        visited.add(nb);
        queue.push(nb);
        treeEdgeSet.add(`${cur}-${nb}`);
        treeEdgeSet.add(`${nb}-${cur}`);
        childrenMap.get(cur)!.push({
          id: nodeId(nb),
          data: { name: charMap.get(nb)!.name },
          children: [],
        });
      }
    }
  }

  const roots: any[] = [
    {
      id: nodeId(rootId),
      data: { name: charMap.get(rootId)!.name },
      children: childrenMap.get(rootId) || [],
    },
  ];
  for (const c of characters) {
    if (!visited.has(c.id))
      roots.push({ id: nodeId(c.id), data: { name: c.name }, children: [] });
  }

  const treeData: any =
    roots.length === 1
      ? roots[0]
      : { id: "__root__", data: { name: "" }, children: roots };

  const nonTreeEdges = validRelations.filter(
    (r) =>
      !treeEdgeSet.has(`${r.source_character_id}-${r.target_character_id}`),
  );

  return { treeData, nonTreeEdges };
}

export default function CharacterGraph({ novelId, focusId }: Props) {
  const app = useApp();
  const { t } = useTranslation();
  const C = useGraphColors();
  const containerRef = useRef<HTMLDivElement>(null);
  const graphRef = useRef<Graph | null>(null);

  const [characters, setCharacters] = useState<character.Character[]>([]);
  const [relations, setRelations] = useState<character.CharacterRelation[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedCharacter, setSelectedCharacter] =
    useState<character.Character | null>(null);
  const [expanded, setExpanded] = useState(false);

  const load = useCallback(async () => {
    if (!novelId) {
      setCharacters([]);
      setRelations([]);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const [charList, relList] = await Promise.all([
        app.GetCharacters(novelId),
        app.GetCharacterRelations(novelId),
      ]);
      setCharacters(charList ?? []);
      setRelations(relList ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("character.loadFailed"));
    } finally {
      setLoading(false);
    }
  }, [app, novelId, t]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    if (focusId && focusId > 0 && characters.length > 0) {
      const char = characters.find((c) => c.id === focusId);
      if (char) setSelectedCharacter(char);
    }
  }, [focusId, characters]);

  const graphData = useMemo(() => {
    const charIds = new Set(characters.map((c) => c.id));
    const { treeData, nonTreeEdges } = buildCharacterTree(
      characters,
      relations,
    );
    if (!treeData) return { nodes: [], edges: [] };

    let baseGraph: any;
    try {
      baseGraph = treeToGraphData(treeData);
    } catch (err) {
      console.error("treeToGraphData failed:", err);
      return { nodes: [], edges: [] };
    }

    const nodes = (baseGraph.nodes ?? []).map((n: any) => {
      const char = characters.find((c) => nodeId(c.id) === n.id);
      if (!char) return { ...n, style: { size: [1, 1], opacity: 0 } };
      return {
        ...n,
        data: { character: char },
        style: {
          size: [80, 34],
          fill: C.nodeFill,
          stroke: C.nodeStroke,
          labelText: char.name,
          labelFill: C.nodeText,
          labelPlacement: "center" as const,
        },
      };
    });

    const treeEdges = (baseGraph.edges ?? [])
      .filter((e: any) => e.source !== "__root__" && e.target !== "__root__")
      .map((e: any) => ({
        ...e,
        type: "polyline" as const,
        style: {
          stroke: C.edge,
          lineWidth: 2,
          endArrow: true,
          endArrowSize: 10,
        },
      }));

    const extraEdges = nonTreeEdges
      .filter(
        (r) =>
          charIds.has(r.source_character_id) &&
          charIds.has(r.target_character_id),
      )
      .map((r, i) => ({
        id: `extra-${r.id || i}`,
        source: nodeId(r.source_character_id),
        target: nodeId(r.target_character_id),
        type: "line" as const,
        data: { relation: r },
        style: {
          stroke: C.edgeDim,
          lineWidth: 1,
          lineDash: [4, 4],
          opacity: r.is_current ? 1 : 0.42,
        },
      }));

    const nodeIds = new Set(nodes.map((n: any) => n.id));
    const allEdges = [...treeEdges, ...extraEdges].filter(
      (e) => nodeIds.has(e.source) && nodeIds.has(e.target),
    );

    return { nodes, edges: allEdges };
  }, [characters, relations, C]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container || loading || characters.length === 0) return;

    let graph: Graph | null = null;
    try {
      graph = new Graph({
        container,
        data: graphData,
        autoFit: "view",
        background: C.bg,
        animation: false,
        layout: {
          type: "dagre",
          rankdir: "LR",
          nodesep: 50,
          ranksep: 120,
        },
        node: {
          type: "rect",
          style: {
            size: [80, 34],
            radius: 17,
            lineWidth: 2,
            cursor: "pointer",
            labelFontSize: 13,
            labelFontWeight: 600,
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
      const char = characters.find((c) => rawId.startsWith(nodeId(c.id)));
      if (char) {
        setSelectedCharacter((prev) => (prev?.id === char.id ? null : char));
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
  }, [characters, C.bg, C.edge, graphData, loading]);

  const treeEdgeCount = (graphData.edges ?? []).filter(
    (e) => e.type === "polyline",
  ).length;
  const extraEdgeCount = (graphData.edges ?? []).filter(
    (e) => e.type === "line",
  ).length;

  return (
    <main className="relative flex-1 min-w-0 overflow-hidden bg-background">
      <div className="absolute left-5 right-5 top-4 z-10 flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2 text-sm font-semibold text-foreground">
            <UsersRound className="h-4 w-4 text-tag-blue-foreground" />
            {t("character.relationGraphTitle")}
          </div>
          <div className="mt-1 text-xs text-muted-foreground">
            {t("character.characterCount", { count: characters.length })} ·{" "}
            {t("character.mainRelationCount", { count: treeEdgeCount })} ·{" "}
            {t("character.otherRelationCount", { count: extraEdgeCount })}
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
            title={t("character.fitView")}
          >
            <LocateFixed className="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            onClick={load}
            className="flex h-7 w-7 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
            title={t("character.refresh")}
          >
            <RefreshCw className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>

      {selectedCharacter &&
        (() => {
          const personality = safeJson<Record<string, any>>(
            selectedCharacter.personality,
            {},
          );
          const abilities = safeJson<string[]>(selectedCharacter.abilities, []);
          const desc = selectedCharacter.description?.trim() || "";
          const brief: string =
            personality?.brief || personality?.brief_description || "";
          const traits: string[] = Array.isArray(personality?.traits)
            ? personality.traits
            : [];
          const hasContent =
            desc || brief || traits.length > 0 || abilities.length > 0;
          const longDesc = desc.length > 100;

          return (
            <div className="absolute left-5 bottom-5 z-10 w-64 rounded-lg border border-border bg-card/94 p-4 shadow-lg backdrop-blur text-sm">
              <div className="flex items-start justify-between gap-2 mb-2">
                <h3 className="font-semibold text-foreground">
                  {selectedCharacter.name}
                </h3>
                <button
                  onClick={() => {
                    setSelectedCharacter(null);
                    setExpanded(false);
                  }}
                  className="shrink-0 rounded p-0.5 text-muted-foreground hover:text-muted-foreground hover:bg-secondary transition-colors"
                >
                  <X className="h-3.5 w-3.5" />
                </button>
              </div>

              {!hasContent ? (
                <p className="text-xs text-muted-foreground">
                  {t("character.noDetailInfo")}
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
                            ? t("character.collapse")
                            : t("character.expand")}
                        </button>
                      )}
                    </div>
                  )}
                  {brief && (
                    <div>
                      <p className="text-xs text-muted-foreground mb-1">
                        {t("character.intro")}
                      </p>
                      <p className="text-xs text-muted-foreground leading-relaxed">
                        {brief}
                      </p>
                    </div>
                  )}
                  {traits.length > 0 && (
                    <div className="flex flex-wrap gap-1">
                      {traits.map((t, i) => (
                        <span
                          key={i}
                          className="inline-block rounded bg-tag-blue px-2 py-0.5 text-xs text-tag-blue-foreground"
                        >
                          {t}
                        </span>
                      ))}
                    </div>
                  )}
                  {abilities.length > 0 && (
                    <div>
                      <p className="text-xs text-muted-foreground mb-1">
                        {t("character.abilities")}
                      </p>
                      <div className="flex flex-wrap gap-1">
                        {abilities.map((a, i) => (
                          <span
                            key={i}
                            className="inline-block rounded bg-secondary px-2 py-0.5 text-xs text-muted-foreground"
                          >
                            {a}
                          </span>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              )}
            </div>
          );
        })()}

      <div className="absolute right-5 bottom-5 z-10 flex items-center gap-3 rounded-md border border-border bg-card/84 px-3 py-2 text-[11px] text-muted-foreground shadow-sm backdrop-blur">
        <span className="inline-flex items-center gap-1.5">
          <span className="h-0.5 w-7 rounded bg-tag-blue-foreground" />
          {t("character.main")}
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="h-0 w-7 border-t border-dashed border-border" />
          {t("character.other")}
        </span>
      </div>

      {error ? (
        <div className="relative z-10 flex h-full items-center justify-center text-sm text-rose-500">
          {error}
        </div>
      ) : loading ? (
        <div className="relative z-10 flex h-full items-center justify-center text-sm text-muted-foreground">
          {t("character.loading")}
        </div>
      ) : characters.length === 0 ? (
        <div className="relative z-10 flex h-full items-center justify-center">
          <div className="text-center">
            <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-tag-blue text-tag-blue-foreground shadow-sm">
              <UsersRound className="h-6 w-6" />
            </div>
            <div className="mt-3 text-sm font-medium text-foreground">
              {t("character.noCharacters")}
            </div>
          </div>
        </div>
      ) : (
        <div ref={containerRef} className="relative z-[1] h-full w-full" />
      )}
    </main>
  );
}

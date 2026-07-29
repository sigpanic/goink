import { useState, useEffect, useCallback, useMemo } from "react";
import { ChevronRight, MapPin, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toastError } from "@/lib/utils";
import { useApp } from "@/hooks/useApp";
import { useRefresh } from "@/hooks/useRefresh";
import type { location } from "@/hooks/useApp";

interface TreeNode {
  location: location.Location;
  children: TreeNode[];
}

interface Props {
  novelId: number;
}

function buildTree(locations: location.Location[]): TreeNode[] {
  const map = new Map<number, TreeNode>();
  const roots: TreeNode[] = [];

  for (const loc of locations) {
    map.set(loc.id, { location: loc, children: [] });
  }
  for (const loc of locations) {
    const node = map.get(loc.id)!;
    if (loc.parent_location_id && map.has(loc.parent_location_id)) {
      map.get(loc.parent_location_id)!.children.push(node);
    } else {
      roots.push(node);
    }
  }

  return roots;
}

export default function LocationList({ novelId }: Props) {
  const app = useApp();
  const { t } = useTranslation();
  const { refreshNonce, bumpRefresh } = useRefresh();

  const [locations, setLocations] = useState<location.Location[]>([]);
  const [expandedIds, setExpandedIds] = useState<Set<number>>(new Set());

  const load = useCallback(async () => {
    if (!novelId) {
      setLocations([]);
      return;
    }
    const list = await app.GetLocations(novelId);
    setLocations(list ?? []);
  }, [novelId, app]);

  useEffect(() => {
    load();
  }, [load, refreshNonce]);

  async function handleDelete(locId: number) {
    if (!confirm(t("location.confirmDeleteWithChildren"))) return;
    try {
      await app.DeleteLocation(novelId, locId);
      await load();
      bumpRefresh();
    } catch (err) {
      toastError(
        t("location.deleteFailed") +
          ": " +
          (err instanceof Error ? err.message : String(err)),
      );
      console.error(err);
    }
  }

  useEffect(() => {
    // 默认展开根节点
    const roots = locations.filter((l) => !l.parent_location_id);
    setExpandedIds(new Set(roots.map((l) => l.id)));
  }, [locations]);

  const tree = useMemo(() => buildTree(locations), [locations]);

  function toggle(id: number) {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function renderNode(node: TreeNode, depth: number) {
    const { location: loc } = node;
    const isExpanded = expandedIds.has(loc.id);
    const hasChildren = node.children.length > 0;
    return (
      <div key={loc.id} className="group">
        <button
          onClick={() => {
            if (hasChildren) toggle(loc.id);
          }}
          className="w-full flex items-center gap-1.5 px-3 py-1.5 text-left hover:bg-muted/50 transition-colors"
          style={{ paddingLeft: `${12 + depth * 16}px` }}
        >
          {hasChildren ? (
            <ChevronRight
              className={`w-3.5 h-3.5 text-muted-foreground shrink-0 transition-transform duration-200 ${isExpanded ? "rotate-90" : ""}`}
            />
          ) : (
            <span className="w-3.5 shrink-0" />
          )}
          <MapPin className="w-3.5 h-3.5 text-muted-foreground shrink-0" />
          <span className="flex-1 text-sm truncate">{loc.name}</span>
          {loc.location_type && (
            <span className="text-[10px] text-muted-foreground/60 shrink-0">
              {loc.location_type}
            </span>
          )}
          <button
            onClick={(e) => {
              e.stopPropagation();
              handleDelete(loc.id);
            }}
            className="shrink-0 p-0.5 rounded text-muted-foreground hover:text-destructive opacity-0 group-hover:opacity-100 transition-opacity"
            title={t("location.delete")}
          >
            <Trash2 className="h-3 w-3" />
          </button>
        </button>
        {isExpanded && hasChildren && (
          <div>
            {node.children.map((child) => renderNode(child, depth + 1))}
          </div>
        )}
      </div>
    );
  }

  return (
    <>
      <div className="flex items-center justify-between px-3 py-2.5 border-b">
        <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          {t("location.location")} ({locations.length})
        </span>
      </div>

      <div className="flex-1 overflow-y-auto overscroll-contain">
        {tree.length === 0 ? (
          <div className="flex items-center justify-center h-full">
            <p className="text-xs text-muted-foreground">
              {t("location.noLocations")}
            </p>
          </div>
        ) : (
          tree.map((node) => renderNode(node, 0))
        )}
      </div>
    </>
  );
}

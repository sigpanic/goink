import { useState, useEffect, useCallback, useMemo } from "react";
import { Search, Plus, Pencil, Trash2, Heart, Store } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toastError } from "@/lib/utils";
import { useApp } from "@/hooks/useApp";
import type { skill } from "@/hooks/useApp";
import SkillContributeDialog from "./SkillContributeDialog";
import SkillMarketplace from "./SkillMarketplace";
import ConfirmDialog from "@/components/ui/ConfirmDialog";

interface Props {
  novelId: number;
  activeSkillName: string | null;
  onSelectSkill: (path: string, title: string, readOnly: boolean) => void;
  onEditSkill: (path: string, title: string, readOnly: boolean) => void;
  onNewSkill: (name: string) => void;
}

function skillPath(name: string, source: string): string {
  switch (source) {
    case "novel":
      return `skills/${name}.md`;
    case "user":
      return `~/.goink/skills/${name}.md`;
    case "builtin":
      return `/builtin/skills/${name}.md`;
    default:
      return `skills/${name}.md`;
  }
}

export default function SkillList({
  novelId,
  activeSkillName,
  onSelectSkill,
  onEditSkill,
  onNewSkill,
}: Props) {
  const app = useApp();
  const { t } = useTranslation();
  const [skills, setSkills] = useState<skill.SkillMeta[]>([]);
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(false);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");
  const [showContribute, setShowContribute] = useState(false);
  const [marketplaceOpen, setMarketplaceOpen] = useState(false);
  const load = useCallback(async () => {
    if (!novelId) {
      setSkills([]);
      return;
    }
    setLoading(true);
    try {
      const list = await app.ListSkills({ novel_id: novelId });
      setSkills(list ?? []);
    } catch (err) {
      console.error("Failed to load skills:", err);
    } finally {
      setLoading(false);
    }
  }, [app, novelId]);

  const handleMarketplaceInstalled = useCallback(() => {
    load();
  }, [load]);

  useEffect(() => {
    load();
  }, [load]);

  const filtered = useMemo(() => {
    if (!search.trim()) return skills;
    const q = search.toLowerCase();
    return skills.filter(
      (s) =>
        s.name.toLowerCase().includes(q) ||
        s.description.toLowerCase().includes(q),
    );
  }, [skills, search]);

  const novelSkills = filtered.filter((s) => s.source === "novel");
  const userSkills = filtered.filter((s) => s.source === "user");
  const builtinSkills = filtered.filter((s) => s.source === "builtin");

  const [deleteTarget, setDeleteTarget] = useState<skill.SkillMeta | null>(
    null,
  );
  const [deleting, setDeleting] = useState(false);

  // 点击删除按钮只记录目标并弹出确认框，真正的删除在 confirmDelete 里执行
  const handleDelete = (s: skill.SkillMeta) => {
    setDeleteTarget(s);
  };

  const confirmDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await app.DeleteSkill({
        novel_id: novelId,
        name: deleteTarget.name,
        source: deleteTarget.source,
      });
      setDeleteTarget(null);
      await load();
    } catch (err) {
      toastError(
        t("skill.deleteFailed") +
          ": " +
          (err instanceof Error ? err.message : String(err)),
      );
      console.error(err);
    } finally {
      setDeleting(false);
    }
  };

  return (
    <>
      <div className="flex items-center justify-between px-3 py-2.5 border-b gap-1">
        <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          {t("skill.skills")} ({skills.length})
        </span>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setMarketplaceOpen(true)}
            className="p-0.5 rounded hover:bg-muted/60 text-muted-foreground hover:text-foreground transition-colors"
            title={t("skill.marketplace.title")}
          >
            <Store className="w-3.5 h-3.5" />
          </button>
          <button
            onClick={() => setShowContribute(true)}
            className="p-0.5 rounded hover:bg-muted/60 text-muted-foreground hover:text-rose-500 transition-colors"
            title={t("skill.contribute")}
          >
            <Heart className="w-3.5 h-3.5" />
          </button>
          <button
            onClick={() => setCreating(true)}
            className="p-0.5 rounded hover:bg-muted/60 text-muted-foreground hover:text-foreground transition-colors"
            title={t("skill.newSkill")}
          >
            <Plus className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>
      {creating && (
        <div className="px-2 py-1.5 border-b flex gap-1">
          <input
            type="text"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && newName.trim()) {
                onNewSkill(newName.trim());
                setCreating(false);
                setNewName("");
              }
              if (e.key === "Escape") {
                setCreating(false);
                setNewName("");
              }
            }}
            onBlur={() => {
              if (!newName.trim()) {
                setCreating(false);
              }
            }}
            placeholder={t("skill.namePlaceholder")}
            autoFocus
            className="flex-1 px-2 py-0.5 text-xs bg-background border rounded outline-none focus:ring-1 focus:ring-ring"
          />
          <button
            onClick={() => {
              if (newName.trim()) {
                onNewSkill(newName.trim());
                setCreating(false);
                setNewName("");
              }
            }}
            disabled={!newName.trim()}
            className="px-2 py-0.5 text-xs text-action-save hover:text-action-save/80 disabled:opacity-50"
          >
            {t("skill.confirm")}
          </button>
        </div>
      )}
      <div className="px-2 py-1.5">
        <div className="relative">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-3 h-3 text-muted-foreground" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t("skill.search")}
            className="w-full pl-7 pr-2 py-1 text-xs bg-muted/40 rounded border-0 outline-none focus:ring-1 focus:ring-ring"
          />
        </div>
      </div>
      <div className="flex-1 overflow-y-auto overscroll-contain">
        {loading ? (
          <div className="flex items-center justify-center py-8 text-xs text-muted-foreground">
            {t("skill.loading")}
          </div>
        ) : skills.length === 0 ? (
          <div className="flex items-center justify-center py-8 text-xs text-muted-foreground">
            {t("skill.noSkills")}
          </div>
        ) : (
          <>
            {novelSkills.length > 0 && (
              <SkillGroup
                title={t("skill.currentNovel")}
                skills={novelSkills}
                activeSkillName={activeSkillName}
                onSelect={onSelectSkill}
                onEdit={onEditSkill}
                onDelete={handleDelete}
              />
            )}
            {userSkills.length > 0 && (
              <SkillGroup
                title={t("skill.userLevel")}
                skills={userSkills}
                activeSkillName={activeSkillName}
                onSelect={onSelectSkill}
                onEdit={onEditSkill}
                onDelete={handleDelete}
              />
            )}
            {builtinSkills.length > 0 && (
              <SkillGroup
                title={t("skill.builtin")}
                skills={builtinSkills}
                activeSkillName={activeSkillName}
                onSelect={onSelectSkill}
                onEdit={onEditSkill}
                onDelete={handleDelete}
              />
            )}
          </>
        )}
      </div>
      <SkillContributeDialog
        open={showContribute}
        onClose={() => setShowContribute(false)}
      />
      <SkillMarketplace
        open={marketplaceOpen}
        onOpenChange={setMarketplaceOpen}
        novelId={novelId}
        onInstalled={handleMarketplaceInstalled}
      />
      <ConfirmDialog
        open={deleteTarget !== null}
        title={t("skill.confirmDeleteSkill")}
        message={
          deleteTarget
            ? `「${deleteTarget.name}」？${t("skill.irreversible")}`
            : ""
        }
        danger
        loading={deleting}
        confirmText={t("common.delete")}
        onClose={() => setDeleteTarget(null)}
        onConfirm={confirmDelete}
      />
    </>
  );
}

function SkillGroup({
  title,
  skills,
  activeSkillName,
  onSelect,
  onEdit,
  onDelete,
}: {
  title: string;
  skills: skill.SkillMeta[];
  activeSkillName: string | null;
  onSelect: (path: string, title: string, readOnly: boolean) => void;
  onEdit: (path: string, title: string, readOnly: boolean) => void;
  onDelete: (s: skill.SkillMeta) => void;
}) {
  const { t } = useTranslation();
  const isBuiltin = skills[0]?.source === "builtin";
  return (
    <div>
      <div className="px-3 py-1.5">
        <span className="text-[10px] font-semibold text-muted-foreground/60 uppercase tracking-wider">
          {title}
        </span>
      </div>
      {skills.map((s) => {
        const path = skillPath(s.name, s.source);
        const display = `${t("skill.skillLabel")}${s.name}`;
        const readOnly = s.source === "builtin";
        const active = activeSkillName === display;
        return (
          <div key={`${s.source}:${s.name}`} className="group relative">
            <button
              onClick={() => onSelect(path, display, readOnly)}
              className={`w-full flex flex-col px-3 py-1.5 text-left hover:bg-muted/50 transition-colors ${
                active ? "bg-muted" : ""
              }`}
            >
              {active && (
                <span className="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-5 bg-primary rounded-r-full" />
              )}
              <span className="text-sm truncate">{s.name}</span>
              {s.description && (
                <span className="text-[11px] text-muted-foreground truncate">
                  {s.description}
                </span>
              )}
            </button>
            {!isBuiltin && (
              <div className="absolute right-2 top-1/2 -translate-y-1/2 flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    onEdit(path, display, readOnly);
                  }}
                  className="p-0.5 rounded hover:bg-muted/60 text-muted-foreground hover:text-foreground transition-colors"
                  title={t("skill.editSkill")}
                >
                  <Pencil className="w-3 h-3" />
                </button>
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    onDelete(s);
                  }}
                  className="p-0.5 rounded hover:bg-muted/60 text-muted-foreground hover:text-destructive transition-colors"
                  title={t("skill.deleteSkill")}
                >
                  <Trash2 className="w-3 h-3" />
                </button>
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

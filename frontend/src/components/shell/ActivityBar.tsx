import type { LucideIcon } from "lucide-react";
import {
  Library,
  List,
  Search,
  Settings,
  Globe,
  Users,
  MapPin,
  GitBranch,
  History,
  GitGraph,
  Eye,
  Wrench,
  Sparkles,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import type { SidebarPanelId } from "@/types/panel";

interface Activity {
  id: SidebarPanelId;
  icon: LucideIcon;
  labelKey: string;
  disabled?: boolean;
}

const activities: Activity[] = [
  { id: "search", icon: Search, labelKey: "shell.search" },
  { id: "novels", icon: Library, labelKey: "shell.bookshelf" },
  { id: "chapters", icon: List, labelKey: "shell.chapters" },
  { id: "preferences", icon: Settings, labelKey: "shell.preference" },
  { id: "novel-settings", icon: Globe, labelKey: "shell.novelSetting" },
  { id: "characters", icon: Users, labelKey: "shell.characters" },
  { id: "locations", icon: MapPin, labelKey: "shell.locations" },
  { id: "storyarcs", icon: GitBranch, labelKey: "shell.arcs" },
  { id: "timeline", icon: History, labelKey: "shell.timeline" },
  { id: "git", icon: GitGraph, labelKey: "shell.gitHistory" },
  { id: "reader", icon: Eye, labelKey: "shell.readerView" },
  { id: "skills", icon: Wrench, labelKey: "shell.skills" },
  { id: "style-samples", icon: Sparkles, labelKey: "shell.extract" },
];

interface Props {
  activeId: SidebarPanelId;
  onSelect: (id: SidebarPanelId) => void;
}

export default function ActivityBar({ activeId, onSelect }: Props) {
  const { t } = useTranslation();

  return (
    <nav className="w-12 flex flex-col items-center py-3 gap-1.5 border-r bg-sidebar select-none cursor-default">
      {activities.map((a, i) => {
        const isActive = a.id === activeId;
        return (
          <div key={a.id}>
            {i === 0 && <div className="w-6 h-px bg-border my-1 mx-auto" />}
            {i === 4 && <div className="w-6 h-px bg-border my-1 mx-auto" />}
            <button
              disabled={a.disabled}
              onClick={() => onSelect(a.id)}
              title={`${t(a.labelKey)}${a.disabled ? t("shell.comingSoon") : ""}`}
              className={`relative w-10 h-10 flex items-center justify-center rounded-lg transition-all duration-200
                focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring
                ${
                  a.disabled
                    ? "text-muted-foreground/40 cursor-not-allowed"
                    : isActive
                      ? "text-foreground bg-muted"
                      : "text-muted-foreground hover:text-foreground hover:bg-muted/60"
                }`}
            >
              {isActive && !a.disabled && (
                <span className="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-5 bg-primary rounded-r-full" />
              )}
              <a.icon className="w-5 h-5" />
            </button>
          </div>
        );
      })}
    </nav>
  );
}

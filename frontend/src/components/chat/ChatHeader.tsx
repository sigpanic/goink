import { History, Plus } from "lucide-react";
import { useTranslation } from "react-i18next";

interface Props {
  onOpenHistory: () => void;
  onNewChat: () => void;
}

export default function ChatHeader({ onOpenHistory, onNewChat }: Props) {
  const { t } = useTranslation();

  return (
    <div className="px-4 py-2.5 border-b shrink-0 flex items-center justify-between select-none">
      <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
        {t("chat.aiChat")}
      </span>
      <div className="flex items-center gap-2">
        <button
          onClick={onOpenHistory}
          className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
        >
          <History className="w-3.5 h-3.5" /> {t("chat.history")}
        </button>
        <button
          onClick={onNewChat}
          className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
        >
          <Plus className="w-3.5 h-3.5" /> {t("chat.newChat")}
        </button>
      </div>
    </div>
  );
}

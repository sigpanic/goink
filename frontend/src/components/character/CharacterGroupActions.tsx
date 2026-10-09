import { DropdownMenu } from "radix-ui";
import { MoreHorizontal, Pencil, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";

interface Props {
  name: string;
  onEdit: () => void;
  onDelete: () => void;
}

export default function CharacterGroupActions({
  name,
  onEdit,
  onDelete,
}: Props) {
  const { t } = useTranslation();
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          aria-label={t("characterGroup.actions", { name })}
          className="mr-1 rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
        >
          <MoreHorizontal className="h-3.5 w-3.5" />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="end"
          sideOffset={4}
          className="z-50 min-w-32 rounded-md border bg-popover p-1 text-popover-foreground shadow-md"
        >
          <DropdownMenu.Item
            onSelect={onEdit}
            className="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-xs outline-none focus:bg-muted"
          >
            <Pencil className="h-3 w-3" />
            {t("common.edit")}
          </DropdownMenu.Item>
          <DropdownMenu.Item
            onSelect={onDelete}
            className="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-xs text-destructive outline-none focus:bg-muted"
          >
            <Trash2 className="h-3 w-3" />
            {t("common.delete")}
          </DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

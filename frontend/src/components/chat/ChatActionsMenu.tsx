import { useRef } from "react";
import { DropdownMenu } from "radix-ui";
import { Plus, ImagePlus } from "lucide-react";
import { useTranslation } from "react-i18next";

interface Props {
  onImportImages: (files: File[]) => Promise<void>;
  uploadDisabled: boolean;
}

export default function ChatActionsMenu({
  onImportImages,
  uploadDisabled,
}: Props) {
  const { t } = useTranslation();
  const fileRef = useRef<HTMLInputElement>(null);

  return (
    <>
      <input
        ref={fileRef}
        type="file"
        accept="image/png,image/jpeg"
        multiple
        hidden
        disabled={uploadDisabled}
        aria-label={t("chat.addImages")}
        onChange={(e) => {
          void onImportImages(Array.from(e.target.files || []));
          e.target.value = "";
        }}
      />
      <DropdownMenu.Root>
        <DropdownMenu.Trigger asChild>
          <button
            type="button"
            aria-label={t("chat.moreActions")}
            title={t("chat.moreActions")}
            className="flex h-[30px] w-[30px] shrink-0 items-center justify-center rounded-lg text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
          >
            <Plus className="h-4 w-4" />
          </button>
        </DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content
            side="top"
            align="start"
            sideOffset={4}
            className="z-50 min-w-32 rounded-md border bg-popover p-1 text-popover-foreground shadow-md"
          >
            <DropdownMenu.Item
              disabled={uploadDisabled}
              onSelect={() => fileRef.current?.click()}
              className="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-xs outline-none focus:bg-muted data-[disabled]:pointer-events-none data-[disabled]:opacity-40"
            >
              <ImagePlus className="h-3.5 w-3.5" />
              {t("chat.addImages")}
            </DropdownMenu.Item>
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
    </>
  );
}

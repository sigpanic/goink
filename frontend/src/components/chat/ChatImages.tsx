import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { X, ImageOff } from "lucide-react";
import PopSelect from "@/components/shared/PopSelect";
import { GetChatImage } from "@/lib/wailsjs/go/app/App";
import type { attachment } from "@/lib/wailsjs/go/models";
import { attachmentKeys } from "@/lib/queryKeys";
import { Dialog } from "radix-ui";

interface Props {
  novelId: number;
  images: attachment.Ref[];
  disabled?: boolean;
  onRemove?: (index: number) => void;
  onVariantChange?: (index: number, variant: string) => void;
}

function ChatImage({
  novelId,
  image,
  disabled,
  onRemove,
  onVariantChange,
}: {
  novelId: number;
  image: attachment.Ref;
  disabled?: boolean;
  onRemove?: () => void;
  onVariantChange?: (variant: string) => void;
}) {
  const { t } = useTranslation();
  const [failed, setFailed] = useState(false);
  const [open, setOpen] = useState(false);
  const metadata = useQuery({
    queryKey: attachmentKeys.detail(novelId, image.attachment_id),
    queryFn: () => GetChatImage(novelId, image.attachment_id),
    enabled: novelId > 0,
    staleTime: Infinity,
    retry: false,
  });
  const name = metadata.data?.name || image.attachment_id;
  const url = `/attachments/${novelId}/${encodeURIComponent(image.attachment_id)}/${encodeURIComponent(image.variant)}`;
  const unavailable = failed || metadata.isError;

  return (
    <div className="relative w-28 shrink-0 rounded-lg border bg-background/60 p-1 text-foreground">
      <button
        type="button"
        onClick={() => setOpen(true)}
        disabled={unavailable}
        aria-label={t("chat.previewImage", { name })}
        className="flex h-20 w-full items-center justify-center overflow-hidden rounded"
      >
        {unavailable ? (
          <ImageOff className="h-6 w-6 text-muted-foreground" />
        ) : (
          <img
            src={url}
            alt={name}
            onError={() => setFailed(true)}
            className="h-full w-full object-contain"
            loading="lazy"
          />
        )}
      </button>
      <div className="truncate px-1 text-xs" title={name}>
        {name}
      </div>
      {unavailable && (
        <p className="px-1 text-xs text-destructive" role="status">
          {t("chat.imageUnavailable")}
        </p>
      )}
      {onRemove && (
        <button
          type="button"
          aria-label={t("chat.removeImage", { name })}
          disabled={disabled}
          onClick={onRemove}
          className="absolute -right-1 -top-1 rounded-full border bg-background p-0.5 disabled:opacity-40"
        >
          <X className="h-3.5 w-3.5" />
        </button>
      )}
      {onVariantChange && (
        <PopSelect
          value={image.variant}
          disabled={disabled}
          onChange={onVariantChange}
          ariaLabel={t("chat.imageQuality", { name })}
          className="mt-1 w-full"
          minWidth="100%"
          options={[
            { value: "standard-v1", label: t("chat.standardImage") },
            { value: "original", label: t("chat.originalImage") },
          ]}
        />
      )}
      {open && (
        <Dialog.Root open={open} onOpenChange={setOpen}>
          <Dialog.Portal>
            <Dialog.Overlay className="fixed inset-0 z-50 bg-black/50" />
            <Dialog.Content
              aria-describedby={undefined}
              className="fixed left-1/2 top-1/2 z-50 w-[90vw] -translate-x-1/2 -translate-y-1/2 rounded-xl border bg-background p-4 shadow-xl"
            >
              <Dialog.Title className="truncate pr-8 text-sm">
                {name}
              </Dialog.Title>
              <Dialog.Close
                aria-label={t("common.close")}
                className="absolute right-3 top-3 rounded p-1"
              >
                <X className="h-4 w-4" />
              </Dialog.Close>
              <img
                src={url}
                alt={name}
                className="max-h-[80vh] w-full object-contain"
              />
            </Dialog.Content>
          </Dialog.Portal>
        </Dialog.Root>
      )}
    </div>
  );
}

export default function ChatImages({
  novelId,
  images,
  disabled,
  onRemove,
  onVariantChange,
}: Props) {
  return (
    <div className="flex flex-wrap gap-2">
      {images.map((image, index) => (
        <ChatImage
          key={`${image.attachment_id}:${image.variant}:${index}`}
          novelId={novelId}
          image={image}
          disabled={disabled}
          onRemove={onRemove ? () => onRemove(index) : undefined}
          onVariantChange={
            onVariantChange
              ? (variant) => onVariantChange(index, variant)
              : undefined
          }
        />
      ))}
    </div>
  );
}

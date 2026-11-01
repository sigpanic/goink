import type { ComponentProps } from "react";
import ChatInput from "./ChatInput";
import ChatControls from "./ChatControls";
import useChatInputDraft from "./useChatInputDraft";

type Props = Omit<ComponentProps<typeof ChatInput>, "draft"> &
  Omit<
    ComponentProps<typeof ChatControls>,
    "onImportImages" | "uploadDisabled"
  > &
  Pick<Parameters<typeof useChatInputDraft>[0], "draftKey" | "onSend">;

export default function ChatComposer({
  novelId,
  supportsVision,
  disabled,
  isLoading,
  isCancelling,
  placeholder,
  draftKey,
  slashItems,
  onSend,
  onStop,
  onListSlash,
  ...controls
}: Props) {
  const draft = useChatInputDraft({ novelId, draftKey, disabled, onSend });

  return (
    <>
      <ChatInput
        novelId={novelId}
        supportsVision={supportsVision}
        disabled={disabled}
        isLoading={isLoading}
        isCancelling={isCancelling}
        placeholder={placeholder}
        slashItems={slashItems}
        onStop={onStop}
        onListSlash={onListSlash}
        draft={draft}
      />
      <div className="border-t mx-4" />
      <ChatControls
        {...controls}
        onImportImages={draft.importImages}
        uploadDisabled={
          disabled || draft.isImporting || draft.images.length >= 4
        }
      />
    </>
  );
}

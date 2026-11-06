import { useRef } from "react";
import { useTranslation } from "react-i18next";
import { Dialog } from "radix-ui";
import AnnualReportView from "./AnnualReportView";
import styles from "./annual-report.module.css";

export default function AnnualReportDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const previousFocus = useRef<HTMLElement | null>(null);
  return (
    <Dialog.Root
      open={open}
      onOpenChange={(value) => {
        if (!value) onClose();
      }}
      modal={false}
    >
      <Dialog.Portal>
        <Dialog.Content
          className={styles["report-fullscreen"]}
          aria-describedby={undefined}
          onInteractOutside={(event) => event.preventDefault()}
          onOpenAutoFocus={() => {
            previousFocus.current =
              document.activeElement instanceof HTMLElement
                ? document.activeElement
                : null;
          }}
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            if (previousFocus.current?.isConnected)
              previousFocus.current.focus();
          }}
          onKeyDown={(event) => {
            if (event.key !== "Tab") return;
            const items = Array.from(
              event.currentTarget.querySelectorAll<HTMLElement>(
                "button:not(:disabled), select, input, a[href], [tabindex='0']",
              ),
            ).filter((item) => !item.closest("[inert]"));
            const first = items[0];
            const last = items[items.length - 1];
            if (event.shiftKey && document.activeElement === first) {
              event.preventDefault();
              last?.focus();
            } else if (!event.shiftKey && document.activeElement === last) {
              event.preventDefault();
              first?.focus();
            }
          }}
        >
          <Dialog.Title className="sr-only">
            {t("annualReport.title")}
          </Dialog.Title>
          <AnnualReportView onBack={onClose} />
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

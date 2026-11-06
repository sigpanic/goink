import { useRef } from "react";
import { Dialog } from "radix-ui";
import { ArrowUpRight, Feather, Sparkle, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import styles from "./annual-report-prompt.module.css";

export default function AnnualReportPromptDialog({
  open,
  year,
  onConfirm,
  onClose,
}: {
  open: boolean;
  year: number;
  onConfirm: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const previousFocus = useRef<HTMLElement | null>(null);
  const confirmButton = useRef<HTMLButtonElement>(null);
  const openingReport = useRef(false);

  return (
    <Dialog.Root
      open={open}
      onOpenChange={(value) => {
        if (!value) onClose();
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className={styles.overlay} />
        <Dialog.Content
          className={styles.dialog}
          onOpenAutoFocus={(event) => {
            previousFocus.current =
              document.activeElement instanceof HTMLElement
                ? document.activeElement
                : null;
            openingReport.current = false;
            event.preventDefault();
            confirmButton.current?.focus();
          }}
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            if (!openingReport.current && previousFocus.current?.isConnected)
              previousFocus.current.focus();
          }}
        >
          <Dialog.Close className={styles.close} aria-label={t("common.close")}>
            <X size={17} />
          </Dialog.Close>

          <div className={styles.art} aria-hidden="true">
            <div className={styles.imprint}>
              <span>Goink</span>
              <i />
              <span>{t("annualReport.reminderBookLabel")}</span>
            </div>
            <div className={styles.orbit} />
            <Sparkle className={styles.sparkle} size={22} strokeWidth={1} />
            <div className={styles.book}>
              <div className={styles.cover}>
                <span className={styles["book-year"]}>{year}</span>
                <Feather size={30} strokeWidth={1} />
                <span className={styles["book-label"]}>
                  {t("annualReport.reminderBookLabel")}
                </span>
                <span className={styles.ribbon} />
              </div>
            </div>
            <span className={styles.year}>{year}</span>
            <span className={styles.caption}>
              {t("annualReport.reminderArtNote")}
            </span>
          </div>

          <div className={styles.content}>
            <p className={styles.kicker}>{t("annualReport.reminderKicker")}</p>
            <Dialog.Title className={styles.title}>
              {t("annualReport.reminderTitle", { year })}
            </Dialog.Title>
            <Dialog.Description className={styles.description}>
              {t("annualReport.reminderNote")}
            </Dialog.Description>
            <div className={styles.actions}>
              <button
                ref={confirmButton}
                type="button"
                className={styles.confirm}
                onClick={() => {
                  openingReport.current = true;
                  onConfirm();
                }}
              >
                {t("annualReport.viewNow")}
                <ArrowUpRight size={17} />
              </button>
              <Dialog.Close className={styles.later}>
                {t("annualReport.viewLater")}
              </Dialog.Close>
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

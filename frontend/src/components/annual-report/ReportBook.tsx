import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { CSSProperties, KeyboardEvent, ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { ArrowLeft, ArrowRight, Feather } from "lucide-react";

interface BookPage {
  id: string;
  label: string;
  content: ReactNode;
}

const TURN_DURATION = 1150;

export default function ReportBook({
  pages,
  title,
  year,
  footer,
}: {
  pages: BookPage[];
  title?: string;
  year?: number;
  footer?: ReactNode;
}) {
  const { t } = useTranslation();
  const [page, setPage] = useState(0);
  const [turn, setTurn] = useState<{
    to: number;
    direction: "next" | "previous";
  } | null>(null);
  const busy = useRef(false);
  const focusAfterTurn = useRef(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const sheet = useRef<HTMLDivElement>(null);
  const scroller = useRef<HTMLDivElement>(null);
  const front = useRef<HTMLDivElement>(null);
  const snapshot = useRef<{ node: HTMLElement; scrollTop: number } | null>(
    null,
  );
  const visiblePage = turn?.to ?? page;

  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    [],
  );
  useLayoutEffect(() => {
    if (!turn && focusAfterTurn.current) {
      focusAfterTurn.current = false;
      sheet.current?.focus({ preventScroll: true });
    }
    const host = front.current;
    const captured = snapshot.current;
    if (!turn || !host || !captured) return;
    host.replaceChildren(captured.node);
    const scroll = captured.node.querySelector(".report-page-scroll");
    if (scroll) scroll.scrollTop = captured.scrollTop;
    snapshot.current = null;
    return () => host.replaceChildren();
  }, [turn]);

  function goTo(next: number) {
    if (busy.current || next < 0 || next >= pages.length || next === page)
      return;
    if (
      window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ||
      !sheet.current
    ) {
      setPage(next);
      if (scroller.current) scroller.current.scrollTop = 0;
      sheet.current?.focus({ preventScroll: true });
      return;
    }
    const node = sheet.current.cloneNode(true) as HTMLElement;
    node.classList.remove("report-sheet-live");
    node.classList.add("report-sheet-snapshot");
    node.removeAttribute("tabindex");
    node.removeAttribute("id");
    node.querySelectorAll("[id]").forEach((item) => item.removeAttribute("id"));
    node.inert = true;
    node.setAttribute("aria-hidden", "true");
    snapshot.current = { node, scrollTop: scroller.current?.scrollTop ?? 0 };
    busy.current = true;
    setTurn({ to: next, direction: next > page ? "next" : "previous" });
    if (scroller.current) scroller.current.scrollTop = 0;
    timer.current = setTimeout(() => {
      focusAfterTurn.current = true;
      setPage(next);
      setTurn(null);
      busy.current = false;
      timer.current = null;
    }, TURN_DURATION);
  }

  function onKeyDown(event: KeyboardEvent) {
    if (
      event.defaultPrevented ||
      event.altKey ||
      event.ctrlKey ||
      event.metaKey
    )
      return;
    const target = event.target as HTMLElement;
    if (
      target.isContentEditable ||
      target.closest(
        "input, select, textarea, .report-share-dialog, .report-calendar-scroll, .report-chart",
      )
    )
      return;
    if (event.key === "ArrowRight" || event.key === "ArrowLeft") {
      event.preventDefault();
      goTo(page + (event.key === "ArrowRight" ? 1 : -1));
    }
  }

  return (
    <section
      className="report-book"
      onKeyDown={onKeyDown}
      aria-label={t("annualReport.bookLabel")}
      aria-busy={turn !== null}
      style={
        { "--report-turn-duration": `${TURN_DURATION}ms` } as CSSProperties
      }
    >
      <div
        className="report-book-frame"
        data-turning={turn !== null}
        data-direction={turn?.direction}
      >
        <div className="report-book-underlay" aria-hidden="true" />
        <div
          className="report-sheet report-sheet-live"
          ref={sheet}
          tabIndex={-1}
          inert={turn !== null}
          aria-label={pages[visiblePage].label}
        >
          <header className="report-paper-heading">
            <span>
              <Feather size={16} />
              Goink
            </span>
            <span>{title ?? t("annualReport.edition")}</span>
            <span>{year}</span>
          </header>
          <div className="report-page-scroll" ref={scroller}>
            <div
              key={pages[visiblePage].id}
              className="report-page-content"
              data-page={pages[visiblePage].id}
            >
              {pages[visiblePage].content}
            </div>
          </div>
          <footer className="report-page-folio">
            <span>{footer ?? "Goink"}</span>
            <i />
            <span>{String(visiblePage + 1).padStart(2, "0")}</span>
          </footer>
        </div>
        {turn && (
          <>
            <div
              className="report-reveal-shadow"
              data-direction={turn.direction}
              aria-hidden="true"
            />
            <div
              className="report-turning-leaf"
              data-direction={turn.direction}
              aria-hidden="true"
              inert
            >
              <div className="report-leaf-front" ref={front} />
              <div className="report-leaf-back">
                <Feather size={35} />
                <span>Goink</span>
                <small>{year}</small>
              </div>
            </div>
          </>
        )}
      </div>
      <nav
        className="report-book-controls"
        aria-label={t("annualReport.pageNavigation")}
      >
        <button
          type="button"
          className="report-page-previous"
          disabled={page === 0 || turn !== null}
          onClick={() => goTo(page - 1)}
        >
          <ArrowLeft size={16} />
          <span>{t("annualReport.previousPage")}</span>
        </button>
        <div className="report-book-progress">
          <nav
            className="report-book-tabs"
            aria-label={t("annualReport.contents")}
          >
            {pages.map((item, index) => (
              <button
                key={item.id}
                type="button"
                disabled={turn !== null}
                aria-current={page === index ? "page" : undefined}
                aria-label={t("annualReport.goToPage", {
                  page: index + 1,
                  title: item.label,
                })}
                onClick={() => goTo(index)}
              >
                <span>{String(index + 1).padStart(2, "0")}</span>
                <b>{item.label}</b>
              </button>
            ))}
          </nav>
          <span aria-live="polite" aria-atomic="true">
            {t("annualReport.pageStatus", {
              page: page + 1,
              total: pages.length,
              title: pages[page].label,
            })}
          </span>
        </div>
        <button
          type="button"
          className="report-page-next"
          disabled={page === pages.length - 1 || turn !== null}
          onClick={() => goTo(page + 1)}
        >
          <span>
            {t(page === 0 ? "annualReport.openBook" : "annualReport.nextPage")}
          </span>
          <ArrowRight size={16} />
        </button>
      </nav>
    </section>
  );
}

import styles from "./report.module.css";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ReportBook from "./ReportBook";

const pages = [
  {
    id: "one",
    label: "One",
    content: (
      <>
        <h2 id="cover-title">First page</h2>
        <input aria-label="Name" />
        <div className={styles["report-calendar-scroll"]}>
          <button type="button">Calendar day</button>
        </div>
      </>
    ),
  },
  { id: "two", label: "Two", content: <h2>Second page</h2> },
  { id: "three", label: "Three", content: <h2>Third page</h2> },
];

beforeEach(() => {
  vi.useFakeTimers();
  vi.stubGlobal("matchMedia", vi.fn().mockReturnValue({ matches: false }));
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("ReportBook", () => {
  it("keeps a complete outgoing page above the next page for one continuous turn", () => {
    const { container } = render(
      <ReportBook pages={pages} title="Creative yearbook" year={2026} />,
    );
    const next = screen.getByRole("button", {
      name: "annualReport.editions.2026.openBook",
    });
    fireEvent.click(next);
    const front = container.querySelector(`.${styles["report-leaf-front"]}`)!;
    expect(front).toHaveTextContent("First page");
    expect(
      front.querySelector(`.${styles["report-paper-heading"]}`),
    ).toHaveTextContent("Creative yearbook");
    expect(
      front.querySelector(`.${styles["report-paper-heading"]}`),
    ).toHaveTextContent("2026");
    expect(front.querySelector("#cover-title")).not.toBeInTheDocument();
    expect(front.closest("[aria-hidden='true']")).toBeInTheDocument();
    expect(
      container.querySelector(`.${styles["report-sheet-live"]}`),
    ).toHaveTextContent("Second page");
    expect(
      container.querySelector(`.${styles["report-sheet-live"]}`),
    ).toHaveAttribute("inert");
    expect(next).toBeDisabled();
    fireEvent.click(next);
    act(() => vi.advanceTimersByTime(800));
    expect(
      container.querySelector(`.${styles["report-turning-leaf"]}`),
    ).toBeInTheDocument();
    expect(next).toBeDisabled();
    act(() => vi.advanceTimersByTime(350));
    expect(
      container.querySelector(`.${styles["report-turning-leaf"]}`),
    ).not.toBeInTheDocument();
    expect(
      container.querySelector(`.${styles["report-sheet-live"]}`),
    ).toHaveFocus();
    expect(
      screen.getByRole("button", {
        name: "annualReport.editions.2026.nextPage",
      }),
    ).not.toBeDisabled();
    fireEvent.click(
      screen.getByRole("button", {
        name: "annualReport.editions.2026.previousPage",
      }),
    );
    expect(
      container.querySelector(`.${styles["report-turning-leaf"]}`),
    ).toHaveAttribute("data-direction", "previous");
    act(() => vi.advanceTimersByTime(1150));
    expect(screen.getByText("First page")).toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: "annualReport.editions.2026.previousPage",
      }),
    ).toBeDisabled();
  });

  it("uses arrow keys without taking them from inputs or the calendar", () => {
    render(<ReportBook pages={pages} />);
    fireEvent.keyDown(screen.getByRole("textbox"), { key: "ArrowRight" });
    fireEvent.keyDown(screen.getByRole("button", { name: "Calendar day" }), {
      key: "ArrowRight",
    });
    expect(
      screen.getByRole("button", {
        name: "annualReport.editions.2026.openBook",
      }),
    ).not.toBeDisabled();
    fireEvent.keyDown(screen.getByRole("region"), { key: "ArrowRight" });
    act(() => vi.advanceTimersByTime(1150));
    expect(screen.getByText("Second page")).toBeInTheDocument();
  });

  it("switches immediately when reduced motion is requested", () => {
    vi.stubGlobal("matchMedia", vi.fn().mockReturnValue({ matches: true }));
    const { container } = render(<ReportBook pages={pages} />);
    fireEvent.click(
      screen.getByRole("button", {
        name: "annualReport.editions.2026.openBook",
      }),
    );
    expect(screen.getByText("Second page")).toBeInTheDocument();
    expect(
      container.querySelector(`.${styles["report-turning-leaf"]}`),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: "annualReport.editions.2026.nextPage",
      }),
    ).not.toBeDisabled();
  });

  it("cleans up an unfinished page turn on unmount", () => {
    const { unmount } = render(<ReportBook pages={pages} />);
    fireEvent.click(
      screen.getByRole("button", {
        name: "annualReport.editions.2026.openBook",
      }),
    );
    expect(vi.getTimerCount()).toBe(1);
    unmount();
    expect(vi.getTimerCount()).toBe(0);
  });
});

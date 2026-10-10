import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import ContributionGrid from "./ContributionGrid";

vi.mock("react-i18next", async () => {
  const { createInstance } = await import("i18next");
  const { default: zhCN } = await import("@/i18n/locales/zh-CN.json");
  const i18n = createInstance();
  await i18n.init({
    lng: "zh-CN",
    resources: { "zh-CN": { translation: zhCN } },
    interpolation: { escapeValue: false },
  });
  return { useTranslation: () => ({ t: i18n.t.bind(i18n), i18n }) };
});

describe("ContributionGrid", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-10-10T12:00:00Z"));
  });

  afterEach(() => vi.useRealTimers());

  it.each([
    { added: 200, deleted: 300, net: -100, level: 2 },
    { added: 120, deleted: 120, net: 0, level: 2 },
    { added: 0, deleted: 80, net: -80, level: 1 },
    { added: 200, deleted: 0, net: 200, level: 2 },
  ])(
    "shows all counts for added=$added deleted=$deleted",
    ({ added, deleted, net, level }) => {
      render(
        <ContributionGrid
          data={{
            "2026-10-08": {
              date: "2026-10-08",
              words_added: added,
              words_deleted: deleted,
              words_net: net,
            },
          }}
        />,
      );
      const day = screen.getByLabelText("2026-10-08");
      expect(day).toHaveClass(`bg-contribution-${level}`);
      fireEvent.mouseEnter(day);
      expect(screen.getByRole("tooltip")).toHaveTextContent(
        `新增 ${added} 字 · 删除 ${deleted} 字 · 净增 ${net > 0 ? "+" : ""}${net} 字`,
      );
      fireEvent.mouseLeave(day);
      expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
    },
  );

  it("shows no writing only when both added and deleted are zero", () => {
    render(<ContributionGrid data={{}} />);
    const day = screen.getByLabelText("2026-10-08");
    expect(day).toHaveClass("bg-contribution-0");
    fireEvent.mouseEnter(day);
    expect(screen.getByRole("tooltip")).toHaveTextContent("无写作");
    expect(screen.getByText("编辑量（新增＋删除）")).toBeInTheDocument();
  });
});

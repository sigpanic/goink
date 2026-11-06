import { useState } from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import AnnualReportPromptDialog from "./AnnualReportPromptDialog";

vi.mock("react-i18next", async () => {
  const { createInstance } = await import("i18next");
  const { default: zhCN } = await import("@/i18n/locales/zh-CN.json");
  const i18n = createInstance();
  await i18n.init({
    lng: "zh-CN",
    resources: { "zh-CN": { translation: zhCN } },
  });
  return { useTranslation: () => ({ t: i18n.t.bind(i18n), i18n }) };
});

function mount() {
  const confirm = vi.fn();
  const close = vi.fn();
  function Harness() {
    const [open, setOpen] = useState(false);
    return (
      <>
        <button onClick={() => setOpen(true)}>打开邀请</button>
        <AnnualReportPromptDialog
          open={open}
          year={2026}
          onConfirm={() => {
            confirm();
            setOpen(false);
          }}
          onClose={() => {
            close();
            setOpen(false);
          }}
        />
      </>
    );
  }
  render(<Harness />);
  const trigger = screen.getByRole("button", { name: "打开邀请" });
  trigger.focus();
  fireEvent.click(trigger);
  return { confirm, close, trigger };
}

describe("AnnualReportPromptDialog", () => {
  it("announces the report year and focuses the primary action", async () => {
    mount();
    const dialog = await screen.findByRole("dialog", {
      name: "你的 2026 年度创作回顾已准备好",
    });
    expect(dialog).toHaveAccessibleDescription(
      "回看这一年的创作足迹。你也可以稍后从个人中心打开年度报告。",
    );
    expect(screen.getByRole("button", { name: "翻开我的年鉴" })).toHaveFocus();
    const later = screen.getByRole("button", { name: "稍后再看" });
    later.focus();
    fireEvent.keyDown(later, { key: "Tab" });
    expect(screen.getByRole("button", { name: "关闭" })).toHaveFocus();
  });

  it("opens the report through the confirm action without calling dismiss", async () => {
    const { confirm, close } = mount();
    fireEvent.click(screen.getByRole("button", { name: "翻开我的年鉴" }));
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    expect(confirm).toHaveBeenCalledOnce();
    expect(close).not.toHaveBeenCalled();
  });

  it.each(["关闭", "稍后再看", "Escape"])(
    "dismisses with %s and restores focus",
    async (action) => {
      const { confirm, close, trigger } = mount();
      if (action === "Escape") fireEvent.keyDown(document, { key: "Escape" });
      else fireEvent.click(screen.getByRole("button", { name: action }));
      await waitFor(() =>
        expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
      );
      await waitFor(() => expect(trigger).toHaveFocus());
      expect(close).toHaveBeenCalledOnce();
      expect(confirm).not.toHaveBeenCalled();
    },
  );
});

import { expect, it } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import ThinkingBlock from "./ThinkingBlock";

it("keeps opaque thinking visible after completion without an expander", () => {
  const { container, rerender } = render(
    <ThinkingBlock content="" hasThinking isStreaming />,
  );
  expect(screen.getByRole("status")).toHaveTextContent("chat.thinking");
  expect(container.querySelector(".animate-spin")).not.toBeNull();
  expect(container.querySelector("details")).toBeNull();

  rerender(<ThinkingBlock content="" hasThinking isStreaming={false} />);
  expect(screen.getByRole("status")).toHaveTextContent("chat.thinkingProcess");
  expect(container.querySelector(".animate-spin")).toBeNull();
  expect(container.querySelector(".thinking-chevron")).toBeNull();
});

it("streams a continuous preview and keeps complete paragraphs collapsed by default", () => {
  const { container, rerender } = render(
    <ThinkingBlock content="先核对人物动机。" isStreaming />,
  );
  const details = container.querySelector("details")!;
  const summary = container.querySelector("summary")!;
  expect(details.open).toBe(false);

  const content = "先核对人物动机。\n\n再检查脚印与凿子的联系。";
  rerender(<ThinkingBlock content={content} isStreaming />);
  expect(container.querySelector(".thinking-preview")).toHaveTextContent(
    "先核对人物动机。 再检查脚印与凿子的联系。",
  );
  expect(details.open).toBe(false);
  fireEvent.click(summary);
  expect(details.open).toBe(true);
  expect(container.querySelector(".thinking-content")?.textContent).toBe(
    content,
  );

  rerender(<ThinkingBlock content={content} isStreaming={false} />);
  expect(container.querySelector(".thinking-preview")).toHaveTextContent(
    "再检查脚印与凿子的联系。",
  );
  expect(details.open).toBe(true);
  expect(container.querySelector(".thinking-shimmer")).toBeNull();
});

it("makes text arriving after an opaque start expandable without opening it", () => {
  const { container, rerender } = render(
    <ThinkingBlock content="" hasThinking isStreaming />,
  );
  rerender(<ThinkingBlock content="正在核对线索" hasThinking isStreaming />);
  expect(container.querySelector("summary")).toHaveTextContent("正在核对线索");
  expect(container.querySelector("details")?.open).toBe(false);
});

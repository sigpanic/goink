import { expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import type { session } from "@/lib/wailsjs/go/models";
import ChatTurn from "./ChatTurn";
import { emptySegment, rebuildTurns, type Turn } from "./types";

vi.mock("@/components/Markdown", () => ({
  default: ({ content }: { content: string }) => <div>{content}</div>,
}));

function makeTurn(status: Turn["status"]): Turn {
  return {
    id: "turn_1",
    turnId: 1,
    userMessage: "检查章节",
    status,
    segments: [
      { ...emptySegment("intro"), content: "先读取章节。" },
      {
        ...emptySegment("read"),
        type: "tool",
        toolId: "call_read",
        toolName: "read",
        toolStatus: "completed",
        displayText: "查看第 14 章",
      },
      {
        ...emptySegment("answer"),
        content: "最终分析结果。",
        thinkingContent: "检查人物动机。",
        hasThinking: true,
        thinkingDone: true,
      },
    ],
  };
}

const callbacks = { onApprove: vi.fn(), onReject: vi.fn() };

it("collapses only on completion and keeps the final answer outside manually expandable history", () => {
  const { rerender } = render(
    <ChatTurn turn={makeTurn("streaming")} {...callbacks} />,
  );
  expect(screen.getByText("先读取章节。")).toBeVisible();
  expect(screen.getByText("查看第 14 章")).toBeVisible();
  expect(screen.getByText("最终分析结果。")).toBeVisible();
  expect(screen.queryByRole("button", { name: "chat.workHistory" })).toBeNull();

  rerender(<ChatTurn turn={makeTurn("done")} {...callbacks} />);
  const toggle = screen.getByRole("button", { name: "chat.workHistory" });
  expect(toggle).toHaveAttribute("aria-expanded", "false");
  expect(screen.getByText("检查章节")).toBeVisible();
  expect(screen.getByText("先读取章节。")).not.toBeVisible();
  expect(screen.getByText("查看第 14 章")).not.toBeVisible();
  expect(
    screen.getByText("检查人物动机。", { selector: ".thinking-preview" }),
  ).not.toBeVisible();
  expect(screen.getAllByText("最终分析结果。")).toHaveLength(1);
  expect(screen.getByText("最终分析结果。")).toBeVisible();

  fireEvent.click(toggle);
  expect(toggle).toHaveAttribute("aria-expanded", "true");
  expect(screen.getByText("先读取章节。")).toBeVisible();
  expect(screen.getByText("查看第 14 章")).toBeVisible();
  expect(
    screen.getByText("检查人物动机。", { selector: ".thinking-preview" }),
  ).toBeVisible();
  rerender(<ChatTurn turn={makeTurn("done")} {...callbacks} />);
  expect(toggle).toHaveAttribute("aria-expanded", "true");
  expect(screen.getAllByText("最终分析结果。")).toHaveLength(1);
  fireEvent.click(toggle);
  expect(screen.getByText("先读取章节。")).not.toBeVisible();
});

it.each(["error", "user_stopped", "system_interrupted"] as const)(
  "leaves the process and outcome visible for %s turns",
  (status) => {
    const { rerender } = render(
      <ChatTurn turn={makeTurn("streaming")} {...callbacks} />,
    );
    rerender(
      <ChatTurn
        turn={{ ...makeTurn(status), errorMessage: "本轮已中断" }}
        {...callbacks}
      />,
    );
    expect(
      screen.queryByRole("button", { name: "chat.workHistory" }),
    ).toBeNull();
    expect(screen.getByText("先读取章节。")).toBeVisible();
    expect(screen.getByText("查看第 14 章")).toBeVisible();
    expect(screen.getByText("最终分析结果。")).toBeVisible();
    expect(screen.getByText("本轮已中断")).toBeVisible();
  },
);

it("defaults to collapsed after rebuilding a completed turn with opaque thinking from history", () => {
  const [turn] = rebuildTurns([
    { role: "user", content: "你好", turn_id: 1, agent_type: "main" },
    {
      role: "assistant",
      content: "你好！",
      has_thinking: true,
      turn_id: 1,
      agent_type: "main",
    },
  ] as session.Message[]);
  render(<ChatTurn turn={turn} {...callbacks} />);
  const toggle = screen.getByRole("button", { name: "chat.workHistory" });
  expect(toggle).toHaveAttribute("aria-expanded", "false");
  expect(screen.getByText("你好！")).toBeVisible();
  expect(screen.queryByRole("status")).toBeNull();
  fireEvent.click(toggle);
  expect(screen.getByRole("status")).toHaveTextContent("chat.thinkingProcess");
});

it("keeps a tool-ending turn expanded rather than treating an earlier reply as the final answer", () => {
  const turn = makeTurn("done");
  turn.segments.pop();
  render(<ChatTurn turn={turn} {...callbacks} />);
  expect(screen.queryByRole("button", { name: "chat.workHistory" })).toBeNull();
  expect(screen.getByText("先读取章节。")).toBeVisible();
  expect(screen.getByText("查看第 14 章")).toBeVisible();
});

it("does not add a history control for a plain answer or hide a thinking-only turn", () => {
  const turn = makeTurn("done");
  turn.segments = [{ ...emptySegment("plain"), content: "直接回答。" }];
  const { rerender } = render(<ChatTurn turn={turn} {...callbacks} />);
  expect(screen.queryByRole("button", { name: "chat.workHistory" })).toBeNull();
  expect(screen.getByText("直接回答。")).toBeVisible();

  turn.segments = [{ ...emptySegment("thought"), hasThinking: true }];
  rerender(<ChatTurn turn={turn} {...callbacks} />);
  expect(screen.queryByRole("button", { name: "chat.workHistory" })).toBeNull();
  expect(screen.getByRole("status")).toHaveTextContent("chat.thinkingProcess");
});

it("keeps manual compression turns visible", () => {
  render(
    <ChatTurn
      turn={{
        ...makeTurn("done"),
        userMessage: "",
        compressionOnly: true,
        segments: [
          {
            ...emptySegment("compression"),
            type: "compression",
            compressionPhase: "done",
          },
        ],
      }}
      {...callbacks}
    />,
  );
  expect(screen.queryByRole("button", { name: "chat.workHistory" })).toBeNull();
  expect(screen.getByText("chat.compressedContext")).toBeVisible();
});

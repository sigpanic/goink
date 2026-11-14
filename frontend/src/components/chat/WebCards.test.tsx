import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import type { session } from "@/lib/wailsjs/go/models";
import WebSearchCard from "./WebSearchCard";
import WebFetchCard from "./WebFetchCard";
import { rebuildTurns } from "./types";

const fixture = vi.hoisted(() => ({ openUrl: vi.fn() }));

vi.mock("@/lib/wailsjs/runtime", () => ({ BrowserOpenURL: fixture.openUrl }));
vi.mock("@/components/Markdown", () => ({
  default: ({ content }: { content: string }) => <div>{content}</div>,
}));

beforeEach(() => {
  vi.clearAllMocks();
});

const legacySearch = {
  queries: ["旧搜索词一", "旧搜索词二"],
  sources: [{ title: "历史来源", url: "https://example.com/legacy" }],
  summary: "历史整体总结",
};

const newSearch = {
  query: "自然语言问题",
  results: [
    {
      title: "新来源一",
      url: "https://example.com/one",
      summary: "第一条网页摘要",
      date: "2026-11-01T00:00:00Z",
      authority: 3,
      rerank_score: 2.382854,
      truncated: true,
    },
    {
      title: "新来源二",
      url: "https://example.com/two",
      summary: "第二条网页摘要",
      truncated: false,
    },
  ],
};

describe("WebSearchCard", () => {
  it("renders new sources with independently expandable summaries", () => {
    render(<WebSearchCard result={newSearch} />);
    expect(screen.getByText("自然语言问题")).toBeVisible();
    expect(screen.getByRole("button", { name: "新来源一" })).toBeVisible();
    expect(screen.getByRole("button", { name: "新来源二" })).toBeVisible();
    expect(screen.getByText("2026-11-01")).toBeVisible();
    expect(screen.getByText("chat.sourceAuthority: 3/4")).toBeVisible();
    expect(screen.getByText("chat.sourceRankScore: 2.38/3")).toBeVisible();
    expect(screen.getByText("chat.summaryTruncated")).toBeVisible();
    expect(screen.queryByText("第一条网页摘要")).toBeNull();
    expect(screen.queryByText("第二条网页摘要")).toBeNull();
    expect(screen.queryByText("chat.searchResultSummary")).toBeNull();

    const buttons = screen.getAllByRole("button", {
      name: "chat.sourceSummary",
    });
    expect(buttons[0]).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(buttons[0]);
    expect(buttons[0]).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("第一条网页摘要")).toBeVisible();
    expect(screen.queryByText("第二条网页摘要")).toBeNull();
    fireEvent.click(buttons[1]);
    expect(screen.getByText("第二条网页摘要")).toBeVisible();
    fireEvent.click(buttons[0]);
    expect(screen.queryByText("第一条网页摘要")).toBeNull();
    expect(screen.getByText("第二条网页摘要")).toBeVisible();
    expect(screen.queryByText("chat.openInBrowser")).toBeNull();
    expect(fixture.openUrl).not.toHaveBeenCalled();
  });

  it("keeps legacy queries, sources, and the overall summary", () => {
    render(<WebSearchCard result={legacySearch} />);
    expect(screen.getByText("旧搜索词一")).toBeVisible();
    expect(screen.getByText("旧搜索词二")).toBeVisible();
    expect(screen.getByRole("button", { name: "历史来源" })).toBeVisible();
    expect(screen.queryByText("chat.sourceSummary")).toBeNull();
    expect(screen.queryByText("chat.sourceAuthority")).toBeNull();
    expect(screen.queryByText("历史整体总结")).toBeNull();
    const toggle = screen.getByRole("button", {
      name: "chat.searchResultSummary",
    });
    fireEvent.click(toggle);
    expect(screen.getByText("历史整体总结")).toBeVisible();
    expect(toggle).toHaveAttribute("aria-expanded", "true");
  });

  it("renders zero scores and hides absent optional fields", () => {
    const { rerender } = render(
      <WebSearchCard
        result={{
          query: "评分测试",
          results: [
            {
              title: "来源",
              url: "https://example.com",
              authority: 0,
              rerank_score: 0,
            },
          ],
        }}
      />,
    );
    expect(screen.getByText("chat.sourceAuthority: 0/4")).toBeVisible();
    expect(screen.getByText("chat.sourceRankScore: 0.00/3")).toBeVisible();
    rerender(
      <WebSearchCard
        result={{
          query: "评分测试",
          results: [{ title: "来源", url: "https://example.com" }],
        }}
      />,
    );
    expect(screen.queryByText(/chat.sourceAuthority/)).toBeNull();
    expect(screen.queryByText(/chat.sourceRankScore/)).toBeNull();
    expect(screen.queryByText("chat.summaryTruncated")).toBeNull();
  });

  it("treats an empty results array as new data without falling back to old sources", () => {
    render(
      <WebSearchCard
        result={{ ...legacySearch, summary: "", query: "无结果", results: [] }}
      />,
    );
    expect(screen.getByText("无结果")).toBeVisible();
    expect(screen.getByText("chat.noSearchResults")).toBeVisible();
    expect(screen.queryByText("历史来源")).toBeNull();
    expect(screen.queryByText("旧搜索词一")).toBeNull();
  });

  it.each([
    ["legacy", legacySearch, "历史来源"],
    ["new", newSearch, "新来源一"],
  ] as const)(
    "renders %s results restored from message history",
    (_, result, title) => {
      const [turn] = rebuildTurns([
        { role: "user", content: "查询资料", turn_id: 1, agent_type: "main" },
        {
          role: "assistant",
          content: "",
          turn_id: 1,
          agent_type: "main",
          extra_metadata: JSON.stringify({
            tool_displays: [
              {
                tool_id: "search",
                tool_name: "web_search",
                phase: "completed",
                result,
              },
            ],
          }),
        },
      ] as unknown as session.Message[]);
      const search = turn.segments.find(
        (segment) => segment.toolName === "web_search",
      );
      expect(search?.result).toBeDefined();
      render(<WebSearchCard result={search!.result!} />);
      expect(screen.getByRole("button", { name: title })).toBeVisible();
    },
  );
});

describe("WebFetchCard", () => {
  it("keeps old fetch results readable without requiring mode metadata", () => {
    render(
      <WebFetchCard
        displayText="抓取网页"
        result={{ url: "https://example.com", title: "旧页面", text: "旧正文" }}
      />,
    );
    expect(screen.getByText("旧页面")).toBeVisible();
    expect(screen.queryByText("chat.fetchLocal")).toBeNull();
    expect(screen.queryByText("chat.fetchRemote")).toBeNull();
    expect(screen.queryByText("chat.contentTruncated")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "chat.pageContent" }));
    expect(screen.getByText("旧正文")).toBeVisible();
  });

  it.each(["local", "remote"])(
    "shows %s mode and truncation before opening the content",
    (mode) => {
      render(
        <WebFetchCard
          displayText="抓取网页"
          result={{
            url: "https://example.com",
            title: "页面",
            text: "截断后的正文",
            mode,
            truncated: true,
          }}
        />,
      );
      expect(
        screen.getByText(
          mode === "local" ? "chat.fetchLocal" : "chat.fetchRemote",
        ),
      ).toBeVisible();
      expect(screen.getByText("chat.contentTruncated")).toBeVisible();
      expect(screen.queryByText("截断后的正文")).toBeNull();
      const toggle = screen.getByRole("button", { name: "chat.pageContent" });
      fireEvent.click(toggle);
      expect(screen.getByText("截断后的正文")).toBeVisible();
      expect(toggle).toHaveAttribute("aria-expanded", "true");
      expect(fixture.openUrl).not.toHaveBeenCalled();
    },
  );
});

it.each(["search", "fetch"])(
  "asks for confirmation before opening a %s source",
  (kind) => {
    const url = "https://example.com/legacy";
    if (kind === "search") {
      render(<WebSearchCard result={legacySearch} />);
    } else {
      render(
        <WebFetchCard
          displayText="抓取网页"
          result={{ title: "历史来源", url, text: "正文" }}
        />,
      );
    }
    const link =
      kind === "search"
        ? screen.getByRole("button", { name: "历史来源" })
        : screen.getByTitle(url);
    fireEvent.click(link);
    expect(
      screen.getByRole("heading", { name: "chat.openInBrowser" }),
    ).toBeVisible();
    expect(fixture.openUrl).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "common.cancel" }));
    expect(
      screen.queryByRole("heading", { name: "chat.openInBrowser" }),
    ).toBeNull();
    expect(fixture.openUrl).not.toHaveBeenCalled();
    fireEvent.click(link);
    fireEvent.click(screen.getByRole("button", { name: "common.confirm" }));
    expect(fixture.openUrl).toHaveBeenCalledExactlyOnceWith(url);
    expect(
      screen.queryByRole("heading", { name: "chat.openInBrowser" }),
    ).toBeNull();
  },
);

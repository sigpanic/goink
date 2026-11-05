import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SaveAnnualReportImage } from "@/lib/wailsjs/go/app/App";
import { apperr } from "@/lib/wailsjs/go/models";
import Report2026 from "./Report2026";
import ReportCalendar from "./ReportCalendar";
import MonthlyChart from "./MonthlyChart";
import { makeReport } from "../../report.fixture";
import { createSharePoster } from "./sharePoster";
import styles from "./report.module.css";

vi.mock("./sharePoster", () => ({ createSharePoster: vi.fn() }));
vi.mock("@/lib/wailsjs/go/app/App", () => ({ SaveAnnualReportImage: vi.fn() }));
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

const currentYear = 2026;
function mount(data = makeReport(2026)) {
  return render(<Report2026 report={data} name="林川" locale="zh-CN" />);
}
beforeEach(() => {
  vi.clearAllMocks();
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date(2026, 10, 4, 12));
  vi.stubGlobal("matchMedia", vi.fn().mockReturnValue({ matches: true }));
  vi.mocked(createSharePoster).mockResolvedValue("data:image/png;base64,cG5n");
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});
describe("2026 yearbook", () => {
  it("shows the recorded totals, coverage, deleted works and model distribution", async () => {
    const { container } = mount();
    expect(
      await screen.findByRole("heading", { name: /故事因你而生长/ }),
    ).toBeInTheDocument();
    expect(screen.getByText("林川的创作年鉴")).toBeInTheDocument();
    expect(screen.getByText(/本年记录/)).toBeInTheDocument();
    const avatar = container.querySelector(
      `.${styles["report-signature"]} img`,
    )!;
    expect(avatar).toHaveAttribute("src", "/avatar");
    fireEvent.error(avatar);
    expect(
      container.querySelector(`.${styles["report-signature"]} img`),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "第 2 页：作品" }));
    expect(screen.getByText("已删除作品")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "第 7 页：搭档" }));
    expect(screen.getByText("provider-a")).toBeInTheDocument();
    const tokens = screen.getByText("已记录 Token 用量").closest("article")!;
    expect(within(tokens).getByText("999")).toBeInTheDocument();
    expect(within(tokens).queryByText("1,050")).not.toBeInTheDocument();
  });

  it.each([true, false])(
    "uses a short, truthful empty review with coverage=%s",
    async (covered) => {
      const data = makeReport();
      data.Activity = {
        NovelsCreated: 0,
        VolumesCreated: 0,
        ChaptersCreated: 0,
        CharactersCreated: 0,
        CharacterGroupsCreated: 0,
        CharacterRelationsCreated: 0,
        LocationsCreated: 0,
        LocationRelationsCreated: 0,
        StoryArcsCreated: 0,
        ArcNodesCreated: 0,
        TimelineEntriesCreated: 0,
        ReaderPerspectivesCreated: 0,
        SettingsCreated: 0,
        PreferencesCreated: 0,
        StyleSamplesCreated: 0,
        SkillsCreated: 0,
        ImagesImported: 0,
        SessionsCreated: 0,
        CreativeSeconds: 0,
        ConversationTurns: 0,
        ToolCalls: 0,
      };
      data.Writing = {
        WordsAdded: 0,
        WordsDeleted: 0,
        WordsNet: 0,
        ActiveDays: 0,
        LongestStreak: 0,
      };
      data.UsageCount = 0;
      data.Models = [];
      data.Novels = [];
      if (!covered) data.Coverage = undefined;
      mount(data);
      await screen.findByRole("heading", { name: /正等待下一次落笔/ });
      expect(
        screen.queryByRole("button", { name: "第 5 页：日历" }),
      ).not.toBeInTheDocument();
      fireEvent.click(screen.getByRole("button", { name: "第 2 页：尾页" }));
      expect(
        await screen.findByText(
          covered ? "这一年的故事，正等你写下" : "这一页，还没有留下记录",
        ),
      ).toBeInTheDocument();
      expect(
        screen.queryByRole("heading", { name: "你最常奔赴的世界" }),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("heading", { name: "一路同行的模型" }),
      ).not.toBeInTheDocument();
    },
  );

  it("presents the new entity totals in their own themed chapters", async () => {
    const { container } = mount();
    await screen.findByRole("heading", { name: /故事因你而生长/ });
    const chapters = [
      ["第 2 页：作品", { 铺开的卷册: "3" }],
      [
        "第 3 页：世界",
        { 建立的角色群体: "2", 建立的角色关系: "12", 建立的地点关系: "5" },
      ],
      [
        "第 4 页：故事",
        { 安排的弧线节点: "18", 记录的时间线条目: "9", 梳理的读者认知: "7" },
      ],
      [
        "第 6 页：灵感",
        {
          记下的创作偏好: "3",
          收藏的风格素材: "8",
          新增与安装的技能: "2",
          导入的图片: "6",
        },
      ],
    ] as const;
    for (const [page, counts] of chapters) {
      fireEvent.click(screen.getByRole("button", { name: page }));
      for (const [label, value] of Object.entries(counts)) {
        const count = screen
          .getByText(label)
          .closest(`.${styles["report-count"]}`)!;
        expect(count.querySelector("dd")).toHaveTextContent(
          new RegExp(`^${value}$`),
        );
      }
      if (page === "第 4 页：故事")
        expect(
          container.querySelector(
            `.${styles["report-story-feature"]} .${styles["report-display-number"]}`,
          ),
        ).toHaveTextContent(/^4$/);
      if (page === "第 6 页：灵感")
        expect(
          container.querySelector(
            `.${styles["report-notebook"]} .${styles["report-display-number"]}`,
          ),
        ).toHaveTextContent(/^15$/);
    }
    fireEvent.click(screen.getByRole("button", { name: "第 7 页：搭档" }));
    const sessions = screen.getByText("开启的会话").closest("article")!;
    expect(
      sessions.querySelector(`.${styles["report-ai-value"]}`),
    ).toHaveTextContent("3");
  });

  it("previews a poster and distinguishes canceled, failed and successful saves", async () => {
    const save = vi.mocked(SaveAnnualReportImage);
    save.mockResolvedValueOnce(
      new apperr.Result_bool_({ data: false, err_code: "" }),
    );
    save.mockResolvedValueOnce(
      new apperr.Result_bool_({
        data: false,
        err_code: "internal",
        err_msg: "disk full",
      }),
    );
    save.mockResolvedValueOnce(
      new apperr.Result_bool_({ data: true, err_code: "" }),
    );
    mount();
    await screen.findByRole("heading", { name: /故事因你而生长/ });
    fireEvent.click(screen.getByRole("button", { name: "第 8 页：尾页" }));
    fireEvent.click(screen.getByRole("button", { name: "生成分享海报" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByRole("img")).toHaveAttribute(
      "src",
      "data:image/png;base64,cG5n",
    );
    fireEvent.keyDown(dialog, { key: "ArrowLeft" });
    expect(screen.getByText("8 / 8 · 尾页")).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: "保存图片" }));
    await waitFor(() =>
      expect(save).toHaveBeenCalledWith({
        year: currentYear,
        png_base64: "cG5n",
      }),
    );
    await waitFor(() =>
      expect(
        within(dialog).getByRole("button", { name: "保存图片" }),
      ).not.toBeDisabled(),
    );
    expect(
      within(dialog).queryByText("已保存到所选位置"),
    ).not.toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: "保存图片" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "图片保存失败，请重试。",
    );
    expect(within(dialog).getByRole("alert")).toHaveTextContent("disk full");
    fireEvent.click(within(dialog).getByRole("button", { name: "保存图片" }));
    expect(await within(dialog).findByRole("status")).toHaveTextContent(
      "已保存到所选位置",
    );
    fireEvent.click(within(dialog).getByRole("button", { name: "关闭预览" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it.each([
    ["report.invalid_year", "报告年份无效，请选择不晚于今年的有效年份"],
    ["report.invalid_image", "分享图片无效，请重新生成海报"],
  ])("localizes poster save error %s", async (code, message) => {
    vi.mocked(SaveAnnualReportImage).mockResolvedValueOnce(
      new apperr.Result_bool_({
        data: false,
        err_code: code,
        err_msg: "backend validation detail",
      }),
    );
    mount();
    await screen.findByRole("heading", { name: /故事因你而生长/ });
    fireEvent.click(screen.getByRole("button", { name: "第 8 页：尾页" }));
    fireEvent.click(screen.getByRole("button", { name: "生成分享海报" }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "保存图片" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(message);
    expect(within(dialog).getByRole("alert")).not.toHaveTextContent(
      "backend validation detail",
    );
  });

  it("reports poster generation failure without trying to save", async () => {
    vi.mocked(createSharePoster).mockRejectedValueOnce(
      new Error("canvas unavailable"),
    );
    mount();
    await screen.findByRole("heading", { name: /故事因你而生长/ });
    fireEvent.click(screen.getByRole("button", { name: "第 8 页：尾页" }));
    fireEvent.click(screen.getByRole("button", { name: "生成分享海报" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "海报生成失败，请重试。",
    );
    expect(screen.getByRole("alert")).toHaveTextContent("canvas unavailable");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(SaveAnnualReportImage).not.toHaveBeenCalled();
  });
});

describe("annual charts", () => {
  it("uses actual monthly time rather than allocating time from word counts", () => {
    render(<MonthlyChart report={makeReport()} />);
    expect(
      screen.getByText("1月，落笔最多的月份 · 900 字"),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "创作时长" }));
    expect(
      screen.getByText("2月，投入最多的月份 · 约 2 小时"),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "1月 · 约 1 小时" }));
    expect(screen.getByText("1月 · 约 1 小时")).toBeInTheDocument();
  });

  it("handles leap days, deleted words, equal-length edits and uncovered dates", () => {
    const data = makeReport(2024);
    data.Coverage = { Start: "2024-02-29", End: "2024-03-02" };
    data.DailyWriting = [
      { date: "2024-02-29", words_added: 0, words_deleted: 80, words_net: -80 },
      {
        date: "2024-03-01",
        words_added: 120,
        words_deleted: 120,
        words_net: 0,
      },
    ];
    const { container } = render(<ReportCalendar report={data} />);
    expect(container.querySelectorAll("[data-date]")).toHaveLength(366);
    const deleted = container.querySelector('[data-date="2024-02-29"]')!;
    expect(deleted).toHaveAttribute("data-level", "1");
    fireEvent.focus(deleted);
    expect(
      screen.getByText(/新增 0 字 · 删除 80 字 · 净增 -80 字/),
    ).toBeInTheDocument();
    expect(container.querySelector('[data-date="2024-03-01"]')).toHaveAttribute(
      "data-level",
      "2",
    );
    expect(
      container.querySelector('[data-date="2024-02-28"]'),
    ).not.toHaveAttribute("data-level");
    expect(
      container.querySelector('[data-date="2024-03-02"]'),
    ).not.toHaveAttribute("data-level");
  });
});

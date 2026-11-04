import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  GetAnnualReport,
  GetSettings,
  GetWritingActivity,
  GetWritingStats,
  SaveAnnualReportImage,
} from "@/lib/wailsjs/go/app/App";
import { apperr, config } from "@/lib/wailsjs/go/models";
import ProfileView from "@/components/profile/ProfileView";
import AnnualReportView from "./AnnualReportView";
import ReportCalendar from "./ReportCalendar";
import MonthlyChart from "./MonthlyChart";
import { makeReport } from "./report.fixture";
import { createSharePoster } from "./sharePoster";

vi.mock("./sharePoster", () => ({ createSharePoster: vi.fn() }));

vi.mock("@/lib/wailsjs/go/app/App", () => ({
  GetAnnualReport: vi.fn(),
  GetSettings: vi.fn(),
  GetWritingActivity: vi.fn(),
  GetWritingStats: vi.fn(),
  SaveAvatar: vi.fn(),
  SaveUserName: vi.fn(),
  SaveAnnualReportImage: vi.fn(),
}));

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

const getReport = vi.mocked(GetAnnualReport);
const currentYear = new Date().getFullYear();
function success(data = makeReport()) {
  return new apperr.Result_github_com_sigpanic_goink_internal_report_AnnualReport_(
    { data, err_code: "" },
  );
}
function mount(view = <AnnualReportView onBack={() => {}} />) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return render(
    <QueryClientProvider client={client}>{view}</QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal("matchMedia", vi.fn().mockReturnValue({ matches: true }));
  vi.mocked(createSharePoster).mockResolvedValue("data:image/png;base64,cG5n");
  getReport.mockImplementation(async (year) => success(makeReport(year)));
  vi.mocked(GetSettings).mockResolvedValue(
    new config.AppSettings({ user_name: "林川" }),
  );
});

afterEach(() => vi.unstubAllGlobals());

describe("AnnualReportView", () => {
  it("shows the recorded totals, coverage, deleted works and model distribution", async () => {
    const back = vi.fn();
    const { container } = mount(<AnnualReportView onBack={back} />);
    expect(screen.getByRole("status")).toBeInTheDocument();
    expect(
      await screen.findByRole("heading", { name: /故事因你而生长/ }),
    ).toBeInTheDocument();
    expect(getReport).toHaveBeenCalledWith(currentYear);
    expect(screen.getByText("林川的创作年鉴")).toBeInTheDocument();
    expect(screen.getByText(/本年记录/)).toBeInTheDocument();
    const avatar = container.querySelector(".report-signature img")!;
    expect(avatar).toHaveAttribute("src", "/avatar");
    fireEvent.error(avatar);
    expect(
      container.querySelector(".report-signature img"),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "第 4 页：作品" }));
    expect(screen.getByText("已删除作品")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "第 5 页：搭档" }));
    expect(screen.getByText("provider-a")).toBeInTheDocument();
    const tokens = screen.getByText("已记录 Token 用量").closest("article")!;
    expect(within(tokens).getByText("999")).toBeInTheDocument();
    expect(within(tokens).queryByText("1,050")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "返回个人中心" }));
    expect(back).toHaveBeenCalledOnce();
  });

  it("does not show a previous year's results while loading or after an older request resolves", async () => {
    let resolvePrevious!: (value: ReturnType<typeof success>) => void;
    getReport.mockImplementation((year) =>
      year === currentYear
        ? Promise.resolve(success())
        : new Promise((resolve) => {
            resolvePrevious = resolve;
          }),
    );
    mount();
    await screen.findByRole("heading", { name: /故事因你而生长/ });
    const select = screen.getByRole("combobox", { name: "查看年份" });
    fireEvent.change(select, { target: { value: String(currentYear - 1) } });
    await waitFor(() =>
      expect(getReport).toHaveBeenCalledWith(currentYear - 1),
    );
    expect(screen.getByRole("status")).toBeInTheDocument();
    expect(screen.queryByText("Night Voyage")).not.toBeInTheDocument();
    fireEvent.change(select, { target: { value: String(currentYear) } });
    await screen.findByRole("heading", { name: /故事因你而生长/ });
    const older = makeReport(currentYear - 1);
    older.Novels[0].Title = "旧年度作品";
    await act(async () => resolvePrevious(success(older)));
    expect(select).toHaveValue(String(currentYear));
    fireEvent.click(screen.getByRole("button", { name: "第 4 页：作品" }));
    expect(screen.queryByText("旧年度作品")).not.toBeInTheDocument();
    expect(screen.getByText("Night Voyage")).toBeInTheDocument();
  });

  it("shows a query failure separately from empty data and supports retry", async () => {
    getReport.mockResolvedValueOnce(
      new apperr.Result_github_com_sigpanic_goink_internal_report_AnnualReport_(
        { err_code: "internal", err_msg: "database unavailable" },
      ),
    );
    mount();
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "暂时无法打开年度报告",
    );
    expect(screen.getByRole("alert")).toHaveTextContent("database unavailable");
    expect(
      screen.queryByText("这一年的故事，正等你写下"),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "重新读取" }));
    expect(
      await screen.findByRole("heading", { name: /故事因你而生长/ }),
    ).toBeInTheDocument();
  });

  it.each([true, false])(
    "uses a short, truthful empty review with coverage=%s",
    async (covered) => {
      const data = makeReport();
      data.Activity = {
        NovelsCreated: 0,
        ChaptersCreated: 0,
        CharactersCreated: 0,
        LocationsCreated: 0,
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
      getReport.mockResolvedValue(success(data));
      mount();
      await screen.findByRole("heading", { name: /正等待下一次落笔/ });
      expect(
        screen.queryByRole("button", { name: "第 3 页：日历" }),
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

  it("keeps the annual report entry hidden on the profile", async () => {
    vi.mocked(GetWritingActivity).mockResolvedValue([]);
    vi.mocked(GetWritingStats).mockResolvedValue({
      total_words: 0,
      total_days_active: 0,
      current_streak: 0,
      longest_streak: 0,
      total_novels: 0,
      total_chapters: 0,
    });
    mount(<ProfileView />);
    await screen.findByRole("heading", { name: "林川" });
    expect(
      screen.queryByRole("button", { name: /你的年度创作回顾/ }),
    ).not.toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(getReport).not.toHaveBeenCalled();
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
    fireEvent.click(screen.getByRole("button", { name: "第 6 页：尾页" }));
    fireEvent.click(screen.getByRole("button", { name: "生成分享海报" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByRole("img")).toHaveAttribute(
      "src",
      "data:image/png;base64,cG5n",
    );
    fireEvent.keyDown(dialog, { key: "ArrowLeft" });
    expect(screen.getByText("6 / 6 · 尾页")).toBeInTheDocument();
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
    fireEvent.click(screen.getByRole("button", { name: "第 6 页：尾页" }));
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
    fireEvent.click(screen.getByRole("button", { name: "第 6 页：尾页" }));
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

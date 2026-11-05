import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  GetAnnualReport,
  GetSettings,
  GetWritingActivity,
  GetWritingStats,
} from "@/lib/wailsjs/go/app/App";
import { apperr, config } from "@/lib/wailsjs/go/models";
import ProfileView from "@/components/profile/ProfileView";
import AnnualReportView from "./AnnualReportView";
import { makeReport } from "./report.fixture";
import { Environment } from "@/lib/wailsjs/runtime/runtime";
import type { AnnualReportEditionProps } from "./types";

vi.mock("@/lib/wailsjs/runtime/runtime", () => ({ Environment: vi.fn() }));
vi.mock("./editions", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./editions")>();
  return {
    ...actual,
    reportEditions: {
      ...actual.reportEditions,
      2027: ({ report }: AnnualReportEditionProps) => (
        <section
          aria-label="2027 alternate edition"
          data-report-year={report.Year}
        >
          <h2>A different annual experience</h2>
        </section>
      ),
    },
  };
});

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
const currentYear = 2026;
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
  vi.mocked(Environment).mockResolvedValue({
    buildType: "production",
    platform: "linux",
    arch: "amd64",
  });
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date(2026, 10, 4, 12));
  vi.stubGlobal("matchMedia", vi.fn().mockReturnValue({ matches: true }));
  getReport.mockImplementation(async (year) => success(makeReport(year)));
  vi.mocked(GetSettings).mockResolvedValue(
    new config.AppSettings({ user_name: "林川" }),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
  vi.useRealTimers();
});

describe("AnnualReportView", () => {
  it("switches between independently registered annual layouts", async () => {
    vi.setSystemTime(new Date(2027, 11, 29));
    mount();
    const edition = await screen.findByRole("region", {
      name: "2027 alternate edition",
    });
    expect(edition).toHaveAttribute("data-report-year", "2027");
    expect(
      screen.queryByRole("heading", { name: /故事因你而生长/ }),
    ).not.toBeInTheDocument();
    const years = screen.getByRole("combobox", { name: "查看年份" });
    fireEvent.change(years, { target: { value: "2026" } });
    await screen.findByRole("heading", { name: /故事因你而生长/ });
    expect(
      screen.queryByRole("region", { name: "2027 alternate edition" }),
    ).not.toBeInTheDocument();
    expect(years).toHaveValue("2026");
  });
  it("keeps the report year and history when an unconfigured year uses the 2026 edition", async () => {
    vi.setSystemTime(new Date(2028, 11, 29));
    getReport.mockImplementation(async (year) => {
      const report = makeReport(year);
      report.TrackingStartDate = "2026-11-05";
      return success(report);
    });
    const back = vi.fn();
    const { container } = mount(<AnnualReportView onBack={back} />);
    await screen.findByRole("heading", { name: /故事因你而生长/ });
    expect(getReport).toHaveBeenCalledWith(2028);
    expect(
      container.querySelector('[data-report-edition="2026"]'),
    ).toBeInTheDocument();
    const years = screen.getByRole("combobox", { name: "查看年份" });
    expect(
      Array.from(years.querySelectorAll("option"), (option) => option.value),
    ).toEqual(["2028", "2027", "2026"]);
    fireEvent.change(years, { target: { value: "2026" } });
    await waitFor(() => expect(getReport).toHaveBeenCalledWith(2026));
    await screen.findByRole("heading", { name: /故事因你而生长/ });
    expect(years).toHaveValue("2026");
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
    fireEvent.click(screen.getByRole("button", { name: "第 2 页：作品" }));
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

  it.each([
    [false, 2026, 10, 4, false],
    [false, 2026, 11, 29, true],
    [false, 2027, 0, 15, true],
    [false, 2027, 0, 16, false],
    [true, 2026, 10, 4, true],
  ])(
    "profile entry respects dev=%s and date %i/%i/%i",
    async (dev, year, month, day, visible) => {
      vi.stubEnv("DEV", dev);
      vi.setSystemTime(new Date(year, month, day));
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
      const entry = screen.queryByRole("button", { name: /你的年度创作回顾/ });
      if (visible) expect(entry).toBeInTheDocument();
      else expect(entry).not.toBeInTheDocument();
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      expect(getReport).not.toHaveBeenCalled();
    },
  );

  it("opens the completed year in January", async () => {
    vi.setSystemTime(new Date(2027, 0, 1));
    mount();
    await screen.findByRole("heading", { name: /故事因你而生长/ });
    expect(getReport).toHaveBeenCalledWith(2026);
    expect(screen.getByRole("combobox", { name: "查看年份" })).toHaveValue(
      "2026",
    );
  });

  it("opens the profile report entry in Wails dev even with a production frontend", async () => {
    vi.stubEnv("DEV", false);
    vi.mocked(Environment).mockResolvedValue({
      buildType: "dev",
      platform: "linux",
      arch: "amd64",
    });
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
    const entry = await screen.findByRole("button", {
      name: /你的年度创作回顾/,
    });
    fireEvent.click(entry);
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    await screen.findByRole("heading", { name: /故事因你而生长/ });
    expect(getReport).toHaveBeenCalledWith(2026);
  });
});

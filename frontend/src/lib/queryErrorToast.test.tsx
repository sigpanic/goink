import { describe, it, expect, beforeEach, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import {
  QueryClient,
  QueryClientProvider,
  useQuery,
} from "@tanstack/react-query";
import type { ReactNode } from "react";

// Mock toastError 捕获调用（用 vi.hoisted 提升，让 vi.mock 工厂能引用）
const { mockToastError, mockI18n } = vi.hoisted(() => ({
  mockToastError: vi.fn(),
  mockI18n: {
    exists: vi.fn(),
    t: vi.fn(),
  },
}));

vi.mock("@/utils/toast", () => ({
  toastError: mockToastError,
}));

// Mock i18n（queryErrorToast 直接 import i18n，不走 useTranslation）
vi.mock("@/i18n", () => ({
  default: mockI18n,
}));

import { installQueryErrorToast } from "./queryErrorToast";

// 每个测试用独立 QueryClient（retry:false 避免重试延迟），无状态残留。
function newClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
}

function makeWrapper(qc: QueryClient) {
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

describe("installQueryErrorToast", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("query error + 有 observer → 触发 toastError", async () => {
    mockI18n.exists.mockReturnValue(true);
    mockI18n.t.mockReturnValue("角色加载失败");

    const qc = newClient();
    const unsub = installQueryErrorToast(qc);

    const { result } = renderHook(
      () =>
        useQuery({
          queryKey: ["characters", 1],
          queryFn: () => {
            throw new Error("fetch failed");
          },
        }),
      { wrapper: makeWrapper(qc) },
    );

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(mockToastError).toHaveBeenCalledOnce();
    expect(mockToastError).toHaveBeenCalledWith("角色加载失败: fetch failed");

    unsub();
  });

  it("query error + 无 observer（fetchQuery）→ 不 toast（observers 判断）", async () => {
    const qc = newClient();
    const unsub = installQueryErrorToast(qc);

    // fetchQuery 是 imperative 调用，不创建 QueryObserver
    await expect(
      qc.fetchQuery({
        queryKey: ["characters", 1],
        queryFn: () => {
          throw new Error("fetch failed");
        },
      }),
    ).rejects.toThrow("fetch failed");

    expect(mockToastError).not.toHaveBeenCalled();

    unsub();
  });

  it("query success → 不 toast", async () => {
    const qc = newClient();
    const unsub = installQueryErrorToast(qc);

    const { result } = renderHook(
      () =>
        useQuery({
          queryKey: ["characters", 1],
          queryFn: () => Promise.resolve([{ id: 1 }]),
        }),
      { wrapper: makeWrapper(qc) },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockToastError).not.toHaveBeenCalled();

    unsub();
  });

  it("已知 prefix 用 i18n 映射（characters → character.charsLoadFailed）", async () => {
    mockI18n.exists.mockReturnValue(true);
    mockI18n.t.mockReturnValue("角色加载失败");

    const qc = newClient();
    const unsub = installQueryErrorToast(qc);

    renderHook(
      () =>
        useQuery({
          queryKey: ["characters", 1],
          queryFn: () => {
            throw new Error("err");
          },
        }),
      { wrapper: makeWrapper(qc) },
    );

    await waitFor(() => expect(mockToastError).toHaveBeenCalled());
    expect(mockI18n.t).toHaveBeenCalledWith("character.charsLoadFailed");

    unsub();
  });

  it("未知 prefix → fallback 到 ${prefix}.loadFailed + ${prefix} load failed 文案", async () => {
    mockI18n.exists.mockReturnValue(false);

    const qc = newClient();
    const unsub = installQueryErrorToast(qc);

    renderHook(
      () =>
        useQuery({
          queryKey: ["unknown-prefix", 1],
          queryFn: () => {
            throw new Error("err");
          },
        }),
      { wrapper: makeWrapper(qc) },
    );

    await waitFor(() => expect(mockToastError).toHaveBeenCalled());
    // i18n.exists 查 unknown-prefix.loadFailed，返回 false → fallback 到 "unknown-prefix load failed"
    expect(mockI18n.exists).toHaveBeenCalledWith("unknown-prefix.loadFailed");
    expect(mockToastError).toHaveBeenCalledWith(
      "unknown-prefix load failed: err",
    );

    unsub();
  });

  it("string error（wails reject）→ toErrorMessage 命中 string 分支", async () => {
    mockI18n.exists.mockReturnValue(true);
    mockI18n.t.mockReturnValue("角色加载失败");

    const qc = newClient();
    const unsub = installQueryErrorToast(qc);

    renderHook(
      () =>
        useQuery({
          queryKey: ["characters", 1],
          queryFn: () => {
            // wails 后端 reject 抛的是 string，不是 Error 对象
            throw "database error";
          },
        }),
      { wrapper: makeWrapper(qc) },
    );

    await waitFor(() => expect(mockToastError).toHaveBeenCalled());
    expect(mockToastError).toHaveBeenCalledWith("角色加载失败: database error");

    unsub();
  });

  it("novel query error → 用 novel.loadFailed 映射", async () => {
    mockI18n.exists.mockReturnValue(true);
    mockI18n.t.mockReturnValue("加载失败");

    const qc = newClient();
    const unsub = installQueryErrorToast(qc);

    renderHook(
      () =>
        useQuery({
          queryKey: ["novels"],
          queryFn: () => {
            throw new Error("db error");
          },
        }),
      { wrapper: makeWrapper(qc) },
    );

    await waitFor(() => expect(mockToastError).toHaveBeenCalled());
    expect(mockI18n.t).toHaveBeenCalledWith("novel.loadFailed");
    expect(mockToastError).toHaveBeenCalledWith("加载失败: db error");

    unsub();
  });
});

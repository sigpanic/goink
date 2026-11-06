import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  GetSettings,
  SaveAvatar,
  SaveUserName,
} from "@/lib/wailsjs/go/app/App";
import { config } from "@/lib/wailsjs/go/models";
import ProfileHeader from "./ProfileHeader";
import { useProfileSettings } from "./useProfileSettings";

vi.mock("@/lib/wailsjs/go/app/App", () => ({
  GetSettings: vi.fn(),
  SaveAvatar: vi.fn(),
  SaveUserName: vi.fn(),
}));

let userName = "";

function ProfileHarness() {
  const settings = useProfileSettings();
  return (
    <ProfileHeader
      userName={settings.data?.user_name ?? ""}
      activityDays={12}
    />
  );
}

function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return render(
    <QueryClientProvider client={client}>
      <ProfileHarness />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.resetAllMocks();
  userName = "林川";
  vi.mocked(GetSettings).mockImplementation(
    async () => new config.AppSettings({ user_name: userName }),
  );
  vi.mocked(SaveUserName).mockImplementation(async (name) => {
    userName = name;
  });
  vi.mocked(SaveAvatar).mockResolvedValue(undefined);
});

describe("ProfileHeader", () => {
  it("saves a trimmed name on Enter and displays the refreshed settings", async () => {
    mount();
    fireEvent.click(await screen.findByRole("heading", { name: "林川" }));
    const input = screen.getByRole("textbox");
    expect(input).toHaveFocus();
    expect(input).toHaveValue("林川");
    fireEvent.change(input, { target: { value: "  新笔名  " } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(
      await screen.findByRole("heading", { name: "新笔名" }),
    ).toBeInTheDocument();
    expect(SaveUserName).toHaveBeenCalledExactlyOnceWith("新笔名");
  });

  it("discards a name draft on Escape without saving it", async () => {
    mount();
    fireEvent.click(await screen.findByRole("heading", { name: "林川" }));
    const input = screen.getByRole("textbox");
    fireEvent.change(input, { target: { value: "未保存昵称" } });
    fireEvent.keyDown(input, { key: "Escape" });
    expect(screen.getByRole("heading", { name: "林川" })).toBeInTheDocument();
    expect(SaveUserName).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("heading", { name: "林川" }));
    expect(screen.getByRole("textbox")).toHaveValue("林川");
  });

  it("keeps a failed name draft editable and allows saving it on blur", async () => {
    vi.mocked(SaveUserName).mockRejectedValueOnce(new Error("昵称保存失败"));
    mount();
    fireEvent.click(await screen.findByRole("heading", { name: "林川" }));
    const input = screen.getByRole("textbox");
    fireEvent.change(input, { target: { value: "新笔名" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(await screen.findByText("昵称保存失败")).toBeInTheDocument();
    expect(input).toHaveValue("新笔名");
    fireEvent.change(input, { target: { value: "重试笔名" } });
    expect(screen.queryByText("昵称保存失败")).not.toBeInTheDocument();
    fireEvent.blur(input);
    expect(
      await screen.findByRole("heading", { name: "重试笔名" }),
    ).toBeInTheDocument();
    expect(SaveUserName).toHaveBeenLastCalledWith("重试笔名");
  });

  it("allows retrying the same avatar after an error and refreshes the image on success", async () => {
    vi.mocked(SaveAvatar).mockRejectedValueOnce(new Error("头像保存失败"));
    const { container } = mount();
    const avatar = container.querySelector("img")!;
    fireEvent.error(avatar);
    expect(container.querySelector("img")).toBeNull();
    const file = new File([new Uint8Array([1, 2, 3])], "avatar.png", {
      type: "image/png",
    });
    Object.defineProperty(file, "arrayBuffer", {
      value: async () => new Uint8Array([1, 2, 3]).buffer,
    });
    const input =
      container.querySelector<HTMLInputElement>('input[type="file"]')!;
    fireEvent.change(input, { target: { files: [file] } });
    expect(await screen.findByText("头像保存失败")).toBeInTheDocument();
    expect(input).toHaveValue("");
    fireEvent.change(input, { target: { files: [file] } });
    await waitFor(() =>
      expect(container.querySelector("img")).toHaveAttribute(
        "src",
        "/avatar?v=1",
      ),
    );
    expect(screen.queryByText("头像保存失败")).not.toBeInTheDocument();
    expect(SaveAvatar).toHaveBeenCalledTimes(2);
    expect(SaveAvatar).toHaveBeenLastCalledWith([1, 2, 3]);
  });
});

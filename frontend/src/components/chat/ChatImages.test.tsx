import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import type { attachment } from "@/lib/wailsjs/go/models";
import ChatImages from "./ChatImages";

const fixture = vi.hoisted(() => ({ getImage: vi.fn() }));
vi.mock("@/lib/wailsjs/go/app/App", () => ({ GetChatImage: fixture.getImage }));

beforeEach(() => {
  vi.clearAllMocks();
  fixture.getImage.mockResolvedValue({
    id: "picture-id",
    name: "reference.png",
  });
});

function renderImages(
  props: {
    onRemove?: (index: number) => void;
    onVariantChange?: (index: number, variant: string) => void;
  } = {},
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const images = [
    { attachment_id: "picture-id", variant: "standard-v1" },
  ] as attachment.Ref[];
  return render(
    <QueryClientProvider client={client}>
      <ChatImages novelId={7} images={images} {...props} />
    </QueryClientProvider>,
  );
}

it("loads a scoped preview and displays an image dialog", async () => {
  renderImages();
  const image = await screen.findByRole("img", { name: "reference.png" });
  expect(fixture.getImage).toHaveBeenCalledWith(7, "picture-id");
  expect(image).toHaveAttribute("src", "/attachments/7/picture-id/standard-v1");
  expect(screen.queryByRole("combobox")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "chat.previewImage" }));
  expect(screen.getByRole("dialog")).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "common.close" }));
  expect(screen.queryByRole("dialog")).toBeNull();
});

it("provides draft removal and quality controls", async () => {
  const onRemove = vi.fn();
  const onVariantChange = vi.fn();
  renderImages({ onRemove, onVariantChange });
  await screen.findByText("reference.png");
  fireEvent.change(screen.getByRole("combobox"), {
    target: { value: "original" },
  });
  expect(onVariantChange).toHaveBeenCalledWith(0, "original");
  fireEvent.click(screen.getByRole("button", { name: "chat.removeImage" }));
  expect(onRemove).toHaveBeenCalledWith(0);
});

it("keeps a missing image placeholder without blocking the rest of the history", async () => {
  renderImages();
  fireEvent.error(await screen.findByRole("img", { name: "reference.png" }));
  expect(screen.getByRole("status")).toHaveTextContent("chat.imageUnavailable");
  expect(
    screen.getByRole("button", { name: "chat.previewImage" }),
  ).toBeDisabled();
  expect(screen.getByText("reference.png")).toBeVisible();
});

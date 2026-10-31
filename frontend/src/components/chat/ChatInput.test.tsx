import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import type { ComponentProps } from "react";
import type { attachment } from "@/lib/wailsjs/go/models";
import ChatInput from "./ChatInput";

const fixture = vi.hoisted(() => ({
  importImage: vi.fn(),
  toastError: vi.fn(),
}));
vi.mock("@/lib/wailsjs/go/app/App", () => ({
  ImportChatImage: fixture.importImage,
}));
vi.mock("@/utils/toast", () => ({ toastError: fixture.toastError }));
vi.mock("./ChatImages", () => ({
  default: ({
    images,
    onRemove,
    onVariantChange,
  }: {
    images: attachment.Ref[];
    onRemove: (index: number) => void;
    onVariantChange: (index: number, variant: string) => void;
  }) => (
    <div>
      {images.map((image, index) => (
        <div key={image.attachment_id}>
          <span>{image.attachment_id}</span>
          <button onClick={() => onRemove(index)}>
            remove {image.attachment_id}
          </button>
          <select
            aria-label={`quality ${image.attachment_id}`}
            value={image.variant}
            onChange={(e) => onVariantChange(index, e.target.value)}
          >
            <option value="standard-v1">standard</option>
            <option value="original">original</option>
          </select>
        </div>
      ))}
    </div>
  ),
}));

function renderInput(extra: Partial<ComponentProps<typeof ChatInput>> = {}) {
  const props: ComponentProps<typeof ChatInput> = {
    novelId: 7,
    supportsVision: true,
    disabled: false,
    isLoading: false,
    isCancelling: false,
    placeholder: "message",
    draftKey: "new_7",
    slashItems: [],
    onSend: vi
      .fn()
      .mockResolvedValue({ success: true, sessionId: "session_7" }),
    onStop: vi.fn(),
    onListSlash: vi.fn(),
    ...extra,
  };
  const view = render(<ChatInput {...props} />);
  const upload = (files: File[]) =>
    fireEvent.change(view.container.querySelector('input[type="file"]')!, {
      target: { files },
    });
  return {
    ...view,
    props,
    upload,
    update: (next: Partial<typeof props>) =>
      view.rerender(<ChatInput {...props} {...next} />),
  };
}

const picture = () =>
  new File(["image bytes"], "picture.png", { type: "image/png" });
const refsKey = "goink_chat_images_";
const textKey = "goink_chat_draft_";

beforeEach(() => {
  localStorage.clear();
  vi.clearAllMocks();
  fixture.importImage.mockResolvedValue({ id: "picture-id" });
});

it("imports files separately, switches quality, and sends an image-only reference", async () => {
  const view = renderInput();
  view.upload([picture()]);
  await screen.findByText("picture-id");
  expect(fixture.importImage).toHaveBeenCalledWith(
    expect.objectContaining({
      novel_id: 7,
      name: "picture.png",
      data: btoa("image bytes"),
    }),
  );
  expect(localStorage.getItem(refsKey + "new_7")).not.toContain(
    btoa("image bytes"),
  );
  fireEvent.change(screen.getByLabelText("quality picture-id"), {
    target: { value: "original" },
  });
  fireEvent.click(screen.getByRole("button", { name: "chat.sendMessage" }));
  expect(view.props.onSend).toHaveBeenCalledWith("", [
    { attachment_id: "picture-id", variant: "original" },
  ]);
  await waitFor(() => expect(screen.queryByText("picture-id")).toBeNull());
  expect(localStorage.getItem(refsKey + "new_7")).toBeNull();
});

it("imports pasted images and removes only their draft references", async () => {
  const view = renderInput();
  const file = picture();
  fireEvent.paste(screen.getByRole("textbox"), {
    clipboardData: {
      items: [{ kind: "file", type: file.type, getAsFile: () => file }],
    },
  });
  await screen.findByText("picture-id");
  fireEvent.click(screen.getByRole("button", { name: "remove picture-id" }));
  expect(screen.queryByText("picture-id")).toBeNull();
  expect(localStorage.getItem(refsKey + "new_7")).toBeNull();
  expect(view.props.onSend).not.toHaveBeenCalled();
});

it("retains text when import fails and blocks sending during an import", async () => {
  let reject!: (error: unknown) => void;
  fixture.importImage.mockReturnValue(
    new Promise((_, fail) => {
      reject = fail;
    }),
  );
  const view = renderInput();
  fireEvent.input(screen.getByRole("textbox"), {
    target: { value: "describe this" },
  });
  view.upload([picture()]);
  await waitFor(() => expect(fixture.importImage).toHaveBeenCalled());
  expect(
    screen.getByRole("button", { name: "chat.sendMessage" }),
  ).toBeDisabled();
  await act(async () => reject("image decode failed"));
  expect(fixture.toastError).toHaveBeenCalledWith("image decode failed");
  expect(screen.getByRole("textbox")).toHaveValue("describe this");
  expect(
    screen.getByRole("button", { name: "chat.sendMessage" }),
  ).toBeEnabled();
});

it("keeps an import in its original novel draft when the user switches novels", async () => {
  let resolve!: (image: { id: string }) => void;
  fixture.importImage.mockReturnValue(
    new Promise((done) => {
      resolve = done;
    }),
  );
  const view = renderInput();
  view.upload([picture()]);
  await waitFor(() => expect(fixture.importImage).toHaveBeenCalled());
  view.update({ novelId: 8, draftKey: "new_8" });
  fireEvent.input(screen.getByRole("textbox"), {
    target: { value: "other novel" },
  });
  await act(async () => resolve({ id: "picture-id" }));
  expect(screen.queryByText("picture-id")).toBeNull();
  expect(screen.getByRole("textbox")).toHaveValue("other novel");
  expect(JSON.parse(localStorage.getItem(refsKey + "new_7")!)).toHaveLength(1);
  expect(localStorage.getItem(refsKey + "new_8")).toBeNull();
  view.update({ novelId: 7, draftKey: "new_7" });
  expect(screen.getByText("picture-id")).toBeVisible();
});

it("restores a failed send into the newly created session draft", async () => {
  let resolve!: (result: { success: boolean; sessionId: string }) => void;
  const onSend = vi.fn().mockReturnValue(
    new Promise((done) => {
      resolve = done;
    }),
  );
  const view = renderInput({ onSend });
  view.upload([picture()]);
  await screen.findByText("picture-id");
  fireEvent.input(screen.getByRole("textbox"), {
    target: { value: "describe this" },
  });
  fireEvent.click(screen.getByRole("button", { name: "chat.sendMessage" }));
  view.update({ draftKey: "session_7" });
  await act(async () => resolve({ success: false, sessionId: "session_7" }));
  expect(screen.getByRole("textbox")).toHaveValue("describe this");
  expect(screen.getByText("picture-id")).toBeVisible();
  expect(localStorage.getItem(textKey + "session_7")).toBe("describe this");
  expect(JSON.parse(localStorage.getItem(refsKey + "session_7")!)).toHaveLength(
    1,
  );
});

it("restores session image drafts and prevents sends to a text-only model", () => {
  localStorage.setItem(
    refsKey + "session_7",
    JSON.stringify([{ attachment_id: "saved-id", variant: "standard-v1" }]),
  );
  const view = renderInput({ draftKey: "session_7", supportsVision: false });
  expect(screen.getByText("saved-id")).toBeVisible();
  expect(screen.getByText("chat.visionRequired")).toBeVisible();
  fireEvent.keyDown(screen.getByRole("textbox"), { key: "Enter" });
  expect(view.props.onSend).not.toHaveBeenCalled();
  expect(
    screen.getByRole("button", { name: "chat.sendMessage" }),
  ).toBeDisabled();
  view.update({ draftKey: "session_8" });
  expect(screen.queryByText("saved-id")).toBeNull();
  view.update({ draftKey: "session_7" });
  expect(screen.getByText("saved-id")).toBeVisible();
});

it("rejects invalid files and limits a batch to four imported images", async () => {
  const view = renderInput();
  const large = picture();
  Object.defineProperty(large, "size", { value: 20 * 1024 * 1024 + 1 });
  view.upload([large, new File(["gif"], "picture.gif", { type: "image/gif" })]);
  await waitFor(() => expect(fixture.toastError).toHaveBeenCalledTimes(2));
  expect(fixture.importImage).not.toHaveBeenCalled();
  let id = 0;
  fixture.importImage.mockImplementation(async () => ({
    id: `picture-${++id}`,
  }));
  view.upload(Array.from({ length: 5 }, picture));
  await screen.findByText("picture-4");
  await waitFor(() =>
    expect(fixture.toastError).toHaveBeenCalledWith("chat.imageLimit"),
  );
  expect(fixture.importImage).toHaveBeenCalledTimes(4);
});

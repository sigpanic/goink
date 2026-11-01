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
import ChatComposer from "./ChatComposer";
import type { SendResult } from "./types";

const fixture = vi.hoisted(() => ({
  importImage: vi.fn(),
  toastError: vi.fn(),
}));
vi.mock("@/lib/wailsjs/go/app/App", () => ({
  ImportChatImage: fixture.importImage,
}));
vi.mock("@/utils/toast", () => ({ toastError: fixture.toastError }));
vi.mock("@/components/settings/useModels", () => ({
  useModels: () => ({ data: [] }),
}));
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

function renderInput(extra: Partial<ComponentProps<typeof ChatComposer>> = {}) {
  const props: ComponentProps<typeof ChatComposer> = {
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
      .mockResolvedValue({
        success: true,
        messageSaved: true,
        sessionId: "session_7",
      }),
    onStop: vi.fn(),
    onListSlash: vi.fn(),
    onSelectModel: vi.fn(),
    onSelectEffort: vi.fn(),
    onToggleApproval: vi.fn(),
    onConfigModel: vi.fn(),
    usage: null,
    ...extra,
  };
  const view = render(<ChatComposer {...props} />);
  const upload = (files: File[]) =>
    fireEvent.change(view.container.querySelector('input[type="file"]')!, {
      target: { files },
    });
  return {
    ...view,
    props,
    upload,
    update: (next: Partial<typeof props>) =>
      view.rerender(<ChatComposer {...props} {...next} />),
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

it("opens the upload picker from the actions menu beside the model selector", async () => {
  const view = renderInput();
  const actions = screen.getByRole("button", { name: "chat.moreActions" });
  const model = screen.getByRole("button", { name: "chat.noModelAvailable" });
  expect(actions.parentElement).toContainElement(model);
  expect(screen.getByRole("textbox").parentElement).not.toContainElement(
    actions,
  );
  expect(
    actions.compareDocumentPosition(model) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
  const input =
    view.container.querySelector<HTMLInputElement>('input[type="file"]')!;
  const openPicker = vi.spyOn(input, "click");

  fireEvent.keyDown(actions, { key: "Enter" });
  fireEvent.click(
    await screen.findByRole("menuitem", { name: "chat.addImages" }),
  );
  expect(openPicker).toHaveBeenCalledOnce();
  expect(fixture.importImage).not.toHaveBeenCalled();

  view.upload([picture()]);
  expect(await screen.findByText("picture-id")).toBeVisible();
});

it("disables the upload item when the composer is disabled", async () => {
  const view = renderInput({ disabled: true });
  fireEvent.keyDown(screen.getByRole("button", { name: "chat.moreActions" }), {
    key: "Enter",
  });
  expect(
    await screen.findByRole("menuitem", { name: "chat.addImages" }),
  ).toHaveAttribute("aria-disabled", "true");
  expect(view.container.querySelector('input[type="file"]')).toBeDisabled();
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
  expect(view.container.querySelector('input[type="file"]')).toBeDisabled();
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

it.each([false, true])(
  "restores a failed send only if the message was not saved (saved=%s)",
  async (messageSaved) => {
    let resolve!: (result: SendResult) => void;
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
    await act(async () =>
      resolve({ success: false, messageSaved, sessionId: "session_7" }),
    );
    if (messageSaved) {
      expect(screen.getByRole("textbox")).toHaveValue("");
      expect(screen.queryByText("picture-id")).toBeNull();
      expect(localStorage.getItem(textKey + "session_7")).toBeNull();
      expect(localStorage.getItem(refsKey + "session_7")).toBeNull();
      return;
    }
    expect(screen.getByRole("textbox")).toHaveValue("describe this");
    expect(screen.getByText("picture-id")).toBeVisible();
    expect(localStorage.getItem(textKey + "session_7")).toBe("describe this");
    expect(
      JSON.parse(localStorage.getItem(refsKey + "session_7")!),
    ).toHaveLength(1);
  },
);

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

it.each(["enter", "button"])(
  "sends text and clears its pending draft through %s",
  async (trigger) => {
    const view = renderInput();
    const textarea = screen.getByRole("textbox");
    fireEvent.input(textarea, { target: { value: "  hello  " } });
    if (trigger === "enter") {
      fireEvent.keyDown(textarea, { key: "Enter" });
    } else {
      fireEvent.click(screen.getByRole("button", { name: "chat.sendMessage" }));
    }
    expect(view.props.onSend).toHaveBeenCalledExactlyOnceWith("hello", []);
    expect(textarea).toHaveValue("");
    expect(localStorage.getItem(textKey + "new_7")).toBeNull();
    await act(async () => {});
    view.update({ draftKey: "session_8" });
    view.update({ draftKey: "new_7" });
    expect(textarea).toHaveValue("");
  },
);

it("saves text before switching drafts and flushes the current draft on unload", () => {
  const view = renderInput();
  const textarea = screen.getByRole("textbox");
  fireEvent.input(textarea, { target: { value: "first draft" } });
  view.update({ draftKey: "session_8" });
  expect(localStorage.getItem(textKey + "new_7")).toBe("first draft");
  expect(textarea).toHaveValue("");
  fireEvent.input(textarea, { target: { value: "second draft" } });
  fireEvent(window, new Event("beforeunload"));
  expect(localStorage.getItem(textKey + "session_8")).toBe("second draft");
  view.update({ draftKey: "new_7" });
  expect(textarea).toHaveValue("first draft");
  view.update({ draftKey: "session_8" });
  expect(textarea).toHaveValue("second draft");
});

it("keeps a new draft when an earlier send fails", async () => {
  let resolve!: (result: SendResult) => void;
  const onSend = vi.fn().mockReturnValue(
    new Promise((done) => {
      resolve = done;
    }),
  );
  renderInput({ onSend });
  const textarea = screen.getByRole("textbox");
  fireEvent.input(textarea, { target: { value: "sent draft" } });
  fireEvent.keyDown(textarea, { key: "Enter" });
  fireEvent.input(textarea, { target: { value: "new draft" } });
  await act(async () =>
    resolve({ success: false, messageSaved: false, sessionId: "" }),
  );
  expect(textarea).toHaveValue("new draft");
});

it("does not restore an unconfirmed send when onSend unexpectedly rejects", async () => {
  renderInput({ onSend: vi.fn().mockRejectedValue("connection lost") });
  fireEvent.input(screen.getByRole("textbox"), {
    target: { value: "sent text" },
  });
  fireEvent.keyDown(screen.getByRole("textbox"), { key: "Enter" });
  await waitFor(() =>
    expect(fixture.toastError).toHaveBeenCalledWith("connection lost"),
  );
  expect(screen.getByRole("textbox")).toHaveValue("");
  expect(localStorage.getItem(textKey + "new_7")).toBeNull();
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
  fireEvent.keyDown(screen.getByRole("button", { name: "chat.moreActions" }), {
    key: "Enter",
  });
  expect(
    await screen.findByRole("menuitem", { name: "chat.addImages" }),
  ).toHaveAttribute("aria-disabled", "true");
});

import { describe, it, expect, vi, beforeEach } from "vitest";

// Mock sonner before importing toastError
vi.mock("sonner", () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
    info: vi.fn(),
    warning: vi.fn(),
  },
}));

import { toast } from "sonner";
import { toastError, cn } from "./utils";

describe("cn", () => {
  it("merges class names", () => {
    expect(cn("foo", "bar")).toBe("foo bar");
  });

  it("handles conditional classes", () => {
    // eslint-disable-next-line no-constant-binary-expression
    expect(cn("base", false && "hidden", "visible")).toBe("base visible");
  });

  it("deduplicates tailwind classes (tailwind-merge)", () => {
    expect(cn("px-2", "px-4")).toBe("px-4");
  });
});

describe("toastError", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls toast.error with the message", () => {
    toastError("something went wrong");
    expect(toast.error).toHaveBeenCalledWith(
      "something went wrong",
      expect.objectContaining({}),
    );
  });

  it("includes a copy action button", () => {
    toastError("error message");
    expect(toast.error).toHaveBeenCalledWith(
      "error message",
      expect.objectContaining({
        action: expect.objectContaining({
          label: "复制",
          onClick: expect.any(Function),
        }),
      }),
    );
  });

  it("uses CSS variables for button style (theme-compatible)", () => {
    toastError("error");
    expect(toast.error).toHaveBeenCalledWith(
      "error",
      expect.objectContaining({
        actionButtonStyle: expect.objectContaining({
          backgroundColor: "var(--primary)",
          color: "var(--primary-foreground)",
        }),
      }),
    );
  });

  it("copy action writes message to clipboard", async () => {
    const writeText = vi.fn();
    Object.assign(navigator, {
      clipboard: { writeText },
    });

    toastError("clipboard test");
    const call = (toast.error as ReturnType<typeof vi.fn>).mock.calls[0];
    const action = call[1].action;
    action.onClick();
    expect(writeText).toHaveBeenCalledWith("clipboard test");
  });
});

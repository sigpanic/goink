import "@testing-library/jest-dom/vitest";
import { vi, afterEach } from "vitest";
import { cleanup } from "@testing-library/react";

// DOM cleanup after each test
afterEach(() => {
  cleanup();
});

// Mock Wails runtime
vi.mock("@/lib/wailsjs/runtime/runtime", () => ({
  EventsOn: vi.fn(),
  EventsOff: vi.fn(),
  EventsEmit: vi.fn(),
  WindowMinimise: vi.fn(),
  WindowToggleMaximise: vi.fn(),
  Quit: vi.fn(),
}));

// Mock Wails App bindings — individual tests can override specific methods
vi.mock(
  "@/lib/wailsjs/go/app/App",
  () =>
    new Proxy(
      {},
      {
        get: () => vi.fn(),
      },
    ),
);

// Mock react-i18next — returns the key as-is
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, options?: Record<string, any>) => {
      if (options) {
        return Object.entries(options).reduce(
          (str, [k, v]) => str.replace(`{{${k}}}`, String(v)),
          key,
        );
      }
      return key;
    },
    i18n: { language: "zh-CN", changeLanguage: vi.fn() },
  }),
  initReactI18next: { type: "3rdParty", init: vi.fn() },
}));

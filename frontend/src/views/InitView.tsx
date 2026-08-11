import { useState, useEffect } from "react";
import { GetPlatform, Initialize } from "@/lib/wailsjs/go/app/App";
import { useTheme, type Theme } from "@/hooks/useTheme";
import { Button } from "@/components/ui/button";
import { Sun, Moon, Languages } from "lucide-react";
import Logo from "@/components/Logo";
import { useTranslation } from "react-i18next";

function ThemePreview({ theme }: { theme: Theme }) {
  const isLight = theme === "light";
  const mockupBg = isLight ? "#ffffff" : "#1b1f2b";
  const sidebarBg = isLight ? "#f3f4f6" : "#111827";
  const line1 = isLight ? "#e5e7eb" : "#374151";
  const line2 = isLight ? "#d1d5db" : "#4b5563";
  const accent = isLight ? "#7c3aed" : "#a78bfa";

  return (
    <div
      className="rounded-lg p-2 mb-3 border border-border"
      style={{ backgroundColor: mockupBg }}
    >
      <div className="flex gap-2">
        <div
          className="w-6 h-14 rounded-sm flex flex-col gap-1 p-1"
          style={{ backgroundColor: sidebarBg }}
        >
          <div
            className="w-4 h-1 rounded-sm"
            style={{ backgroundColor: accent }}
          />
          <div
            className="w-3 h-1 rounded-sm mt-0.5"
            style={{ backgroundColor: line1 }}
          />
          <div
            className="w-3 h-1 rounded-sm"
            style={{ backgroundColor: line1 }}
          />
        </div>
        <div className="flex-1 flex flex-col gap-1.5 pt-1">
          <div
            className="h-1.5 w-3/4 rounded-sm"
            style={{ backgroundColor: line1 }}
          />
          <div
            className="h-1.5 w-1/2 rounded-sm"
            style={{ backgroundColor: line2 }}
          />
          <div
            className="h-1.5 w-2/3 rounded-sm"
            style={{ backgroundColor: line2 }}
          />
          <div className="flex gap-1 mt-1">
            <div
              className="h-2 w-11 rounded-sm"
              style={{ backgroundColor: accent, opacity: 0.85 }}
            />
            <div
              className="h-2 w-7 rounded-sm border"
              style={{ borderColor: line1, backgroundColor: "transparent" }}
            />
          </div>
        </div>
      </div>
    </div>
  );
}

interface Props {
  onInitialized: () => void;
}

export default function InitView({ onInitialized }: Props) {
  const { t, i18n } = useTranslation();
  const { theme, setTheme } = useTheme();

  const THEME_OPTIONS: { key: Theme; icon: React.ReactNode; label: string }[] =
    [
      {
        key: "light",
        icon: <Sun className="w-5 h-5" />,
        label: t("init.lightMode"),
      },
      {
        key: "dark",
        icon: <Moon className="w-5 h-5" />,
        label: t("init.darkMode"),
      },
    ];
  const [selectedTheme, setSelectedTheme] = useState<Theme>(theme);
  const [selectedLang, setSelectedLang] = useState(i18n.language);
  const [dataDir, setDataDir] = useState("");
  const [error, setError] = useState("");
  const [initializing, setInitializing] = useState(false);

  useEffect(() => {
    GetPlatform().then((info) => {
      if (info.defaultPath) setDataDir(info.defaultPath as string);
    });
  }, []);

  function handleThemeSelect(t: Theme) {
    setSelectedTheme(t);
    setTheme(t);
  }

  async function handleInit() {
    setError("");
    setInitializing(true);
    try {
      await Initialize(dataDir);
      onInitialized();
    } catch (e) {
      setError(String(e));
      setInitializing(false);
    }
  }

  return (
    <div className="flex items-center justify-center min-h-screen">
      <div className="w-full max-w-lg mx-auto px-8 py-12 text-center">
        <Logo className="h-16 w-16 mx-auto mb-8" />

        <h1 className="text-3xl font-semibold tracking-tight mb-3">
          {t("init.welcome")}
        </h1>

        <p className="text-base text-muted-foreground mb-8">
          {t("init.subtitle")}
        </p>

        {/* 主题选择 */}
        <div className="mb-8">
          <p className="text-sm text-muted-foreground mb-3">
            {t("init.chooseTheme")}
          </p>
          <div className="grid grid-cols-2 gap-3">
            {THEME_OPTIONS.map((opt) => {
              const selected = selectedTheme === opt.key;
              return (
                <button
                  key={opt.key}
                  onClick={() => handleThemeSelect(opt.key)}
                  className={`
                    relative rounded-xl border-2 p-3 text-left transition-all cursor-pointer
                    ${
                      selected
                        ? "border-primary ring-2 ring-primary/20"
                        : "border-border hover:border-muted-foreground/50 hover:-translate-y-0.5 hover:shadow-md"
                    }
                  `}
                >
                  <ThemePreview theme={opt.key} />
                  <div className="flex items-center gap-2">
                    <span
                      className={
                        selected ? "text-primary" : "text-muted-foreground"
                      }
                    >
                      {opt.icon}
                    </span>
                    <span className="text-sm font-medium">{opt.label}</span>
                    {selected && (
                      <span className="ml-auto w-4 h-4 rounded-full bg-primary flex items-center justify-center">
                        <span className="w-1.5 h-1.5 rounded-full bg-background" />
                      </span>
                    )}
                  </div>
                </button>
              );
            })}
          </div>
        </div>

        {/* 语言选择 */}
        <div className="mb-8">
          <p className="text-sm text-muted-foreground mb-3">
            {t("init.chooseLanguage")}
          </p>
          <div className="grid grid-cols-2 gap-3">
            {(
              [
                { key: "zh-CN", label: "中文", desc: "简体中文" },
                { key: "en", label: "English", desc: "English" },
              ] as const
            ).map((opt) => {
              const selected =
                selectedLang === opt.key ||
                (opt.key === "zh-CN" && selectedLang.startsWith("zh"));
              return (
                <button
                  key={opt.key}
                  onClick={() => {
                    setSelectedLang(opt.key);
                    i18n.changeLanguage(opt.key);
                  }}
                  className={`
                    rounded-xl border-2 p-3 text-left transition-all cursor-pointer
                    ${
                      selected
                        ? "border-primary ring-2 ring-primary/20"
                        : "border-border hover:border-muted-foreground/50 hover:-translate-y-0.5 hover:shadow-md"
                    }
                  `}
                >
                  <div className="flex items-center gap-2.5">
                    <Languages
                      className={`w-5 h-5 ${selected ? "text-primary" : "text-muted-foreground"}`}
                    />
                    <div>
                      <div className="text-sm font-medium">{opt.label}</div>
                      <div className="text-[11px] text-muted-foreground">
                        {opt.desc}
                      </div>
                    </div>
                    {selected && (
                      <span className="ml-auto w-4 h-4 rounded-full bg-primary flex items-center justify-center">
                        <span className="w-1.5 h-1.5 rounded-full bg-background" />
                      </span>
                    )}
                  </div>
                </button>
              );
            })}
          </div>
        </div>

        <div className="bg-muted/40 rounded-lg px-5 py-4 mb-3 text-left">
          <p className="text-xs text-muted-foreground mb-1">
            {t("init.dataDirHint")}
          </p>
          <p className="text-sm font-mono break-all">
            {dataDir || t("init.loading")}
          </p>
        </div>

        <p className="text-xs text-muted-foreground mb-10">
          {t("init.backupNote")}
        </p>

        {error && <p className="text-sm text-destructive mb-6">{error}</p>}

        <Button
          size="lg"
          className="w-full"
          onClick={handleInit}
          disabled={!dataDir || initializing}
        >
          {initializing ? t("init.initializing") : t("init.start")}
        </Button>
      </div>
    </div>
  );
}

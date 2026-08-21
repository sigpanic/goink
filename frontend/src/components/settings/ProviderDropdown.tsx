import { useState, useRef, useEffect, type ReactNode } from "react";
import { ChevronDown } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { llm } from "@/lib/wailsjs/go/models";

interface Props {
  providers: llm.ProviderView[];
  selectedKey: string;
  onSelect: (key: string) => void;
  // icon 由调用方决定渲染：Builtin 传 ProviderIcon，Custom 传首字母圆形
  renderIcon: (key: string, name: string) => ReactNode;
  // 失败红点 badge：testResults[key].ok === false 时显示
  testResults?: Record<
    string,
    { ok: boolean; msg?: string; warning?: string } | undefined
  >;
}

/**
 * 服务商选择下拉：自定义 button + dropdown 列表。
 * Builtin 和 Custom 共用，差异通过 renderIcon 注入（Builtin 用 ProviderIcon，
 * Custom 用首字母圆形 fallback）。
 *
 * 列表项右侧失败红点 badge：testResults[key].ok === false 时显示，
 * 让用户在切换 provider 时一眼看到哪些是失败的。
 *
 * 注意：dropdownOpen 在 selectedKey 变化时自动重置（切换后收起列表）。
 */
export default function ProviderDropdown({
  providers,
  selectedKey,
  onSelect,
  renderIcon,
  testResults,
}: Props) {
  const { t } = useTranslation();
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const provider = providers.find((p) => p.key === selectedKey);

  // 切换服务商时重置折叠和下拉状态
  useEffect(() => {
    setDropdownOpen(false);
  }, [selectedKey]);

  // 点击外部关闭下拉
  useEffect(() => {
    if (!dropdownOpen) return;
    const handle = (e: MouseEvent) => {
      if (
        dropdownRef.current &&
        !dropdownRef.current.contains(e.target as Node)
      ) {
        setDropdownOpen(false);
      }
    };
    document.addEventListener("mousedown", handle);
    return () => document.removeEventListener("mousedown", handle);
  }, [dropdownOpen]);

  if (!provider) return null;

  return (
    <div className="relative flex-1" ref={dropdownRef}>
      <button
        onClick={() => setDropdownOpen(!dropdownOpen)}
        className="flex items-center gap-2 w-full h-8 rounded-md border bg-background px-2.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
      >
        {renderIcon(provider.key, provider.name)}
        <span className="flex-1 text-left">{provider.name}</span>
        <ChevronDown
          className={`w-3.5 h-3.5 shrink-0 text-muted-foreground transition-transform duration-200 ${dropdownOpen ? "rotate-180" : ""}`}
        />
      </button>
      {dropdownOpen && (
        <div className="absolute z-50 top-full left-0 right-0 mt-1 rounded-md border bg-popover text-popover-foreground shadow-md py-1 max-h-56 overflow-auto">
          {providers.map((p) => {
            const tr = testResults?.[p.key];
            // 状态 badge 五态：未配置灰 / 未测试灰 / 通过绿 / 通过(限流)黄 / 失败红
            // 颜色全部走主题色变量（--muted-foreground / --success-foreground / --warning-foreground / --destructive）
            // 不赋初值：所有分支都会赋值，初始 "" 会被 ESLint no-useless-assignment 标记
            let badgeColor: string;
            let badgeLabel: string;
            if (!p.api_key) {
              badgeColor = "bg-muted-foreground";
              badgeLabel = t("settings.notConfigured");
            } else if (!tr) {
              badgeColor = "bg-muted-foreground";
              badgeLabel = t("settings.notTested");
            } else if (tr.ok) {
              if (tr.warning) {
                badgeColor = "bg-warning-foreground";
                badgeLabel = t("settings.passedWithWarning");
              } else {
                badgeColor = "bg-success-foreground";
                badgeLabel = t("settings.passed");
              }
            } else {
              badgeColor = "bg-destructive";
              badgeLabel = t("settings.connectionTestFailed");
            }
            return (
              <button
                key={p.key}
                onClick={() => {
                  onSelect(p.key);
                  setDropdownOpen(false);
                }}
                className={`flex items-center gap-2 w-full px-2.5 py-1.5 text-sm hover:bg-muted/50 transition-colors ${p.key === selectedKey ? "bg-muted/30" : ""}`}
              >
                {renderIcon(p.key, p.name)}
                <span>{p.name}</span>
                <span
                  className={`ml-auto w-2 h-2 rounded-full ${badgeColor} shrink-0`}
                  aria-label={badgeLabel}
                />
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}

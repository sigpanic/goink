import { useState, useRef, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { ChevronUp } from "lucide-react";

interface Option {
  value: string;
  label: string;
}

interface FooterAction {
  label: string;
  onClick: () => void;
}

interface Props {
  value: string;
  options: Option[];
  onChange: (value: string) => void;
  onOpen?: () => void;
  className?: string;
  minWidth?: string;
  placeholder?: string;
  footerAction?: FooterAction;
  dropUp?: boolean; // true=向上弹出(默认), false=向下弹出
  disabled?: boolean;
  ariaLabel?: string;
  size?: "compact" | "form";
}

export default function PopSelect({
  value,
  options,
  onChange,
  onOpen,
  className = "",
  minWidth = "130px",
  placeholder,
  footerAction,
  dropUp = true,
  disabled = false,
  ariaLabel,
  size = "compact",
}: Props) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);

  const handleToggle = () => {
    if (disabled) return;
    if (!open && onOpen) onOpen();
    setOpen(!open);
  };

  useEffect(() => {
    if (!open) return;
    const handleClick = (e: MouseEvent) => {
      if (
        containerRef.current &&
        !containerRef.current.contains(e.target as Node)
      ) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [open]);

  const selected = options.find((o) => o.value === value);

  return (
    <div
      ref={containerRef}
      className={`relative ${className}`}
      onKeyDown={(event) => {
        if (event.key === "Escape") {
          event.preventDefault();
          setOpen(false);
          triggerRef.current?.focus();
        }
      }}
    >
      <button
        ref={triggerRef}
        type="button"
        onClick={handleToggle}
        style={{ minWidth }}
        disabled={disabled}
        aria-label={ariaLabel}
        aria-expanded={open}
        aria-haspopup="listbox"
        className={`flex items-center justify-between gap-2 border bg-background text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 ${size === "form" ? "h-9 w-full rounded-md px-3 text-sm text-foreground" : "h-[30px] rounded-lg px-2.5 text-xs text-muted-foreground"}`}
      >
        <span className="truncate">
          {selected?.label || placeholder || t("chat.noModelAvailable")}
        </span>
        <ChevronUp
          className={`w-3 h-3 shrink-0 transition-transform ${open === dropUp ? "rotate-180" : ""}`}
        />
      </button>

      {open && (
        <div
          role="listbox"
          aria-label={ariaLabel}
          className={`absolute left-0 z-50 max-h-64 w-full overflow-y-auto rounded-lg border bg-background shadow-lg ${dropUp ? "bottom-full mb-1" : "top-full mt-1"}`}
        >
          {options.map((opt) => (
            <button
              key={opt.value}
              type="button"
              role="option"
              aria-selected={opt.value === value}
              onClick={() => {
                onChange(opt.value);
                setOpen(false);
              }}
              className={`w-full px-2.5 text-left hover:bg-muted transition-colors ${size === "form" ? "py-2 text-sm" : "py-1.5 text-xs"} ${
                opt.value === value
                  ? "bg-primary/10 text-primary font-medium"
                  : "text-muted-foreground"
              }`}
            >
              {opt.label}
            </button>
          ))}
          {footerAction && (
            <>
              <div className="border-t my-0.5" />
              <button
                type="button"
                onClick={() => {
                  footerAction.onClick();
                  setOpen(false);
                }}
                className="w-full text-left px-2.5 py-1.5 text-xs text-muted-foreground hover:bg-muted transition-colors"
              >
                ⚙ {footerAction.label}
              </button>
            </>
          )}
        </div>
      )}
    </div>
  );
}

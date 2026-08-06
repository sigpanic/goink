import { forwardRef } from "react";
import { Search, X, Loader2 } from "lucide-react";

interface SearchInputProps {
  value: string;
  onChange: (value: string) => void;
  onKeyDown?: (e: React.KeyboardEvent) => void;
  placeholder?: string;
  loading?: boolean;
  /** 传给内部 relative div（如 "flex-1"），外层 padding/border 由调用方控制 */
  className?: string;
}

// SearchInput: 统一搜索框样式（全局搜索 + 领域内搜索共用）。
// 采用 SearchPanel 的样式标准：图标 w-3.5 + input h-7 + 可选清除按钮。
// 调用方负责外层 wrapper（padding/border），此组件只管 input 区域。
const SearchInput = forwardRef<HTMLInputElement, SearchInputProps>(
  ({ value, onChange, onKeyDown, placeholder, loading, className }, ref) => {
    return (
      <div className={`relative ${className ?? ""}`}>
        <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground" />
        <input
          ref={ref}
          type="text"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={onKeyDown}
          placeholder={placeholder}
          className="w-full h-7 rounded-md border bg-background pl-7 pr-7 text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
        {(value || loading) && (
          <button
            onMouseDown={(e) => {
              e.preventDefault(); // 阻止 button 获取焦点，让 input 保持焦点
              onChange("");
            }}
            className="absolute right-1.5 top-1/2 -translate-y-1/2 w-4 h-4 flex items-center justify-center text-muted-foreground hover:text-foreground"
          >
            {loading ? (
              <Loader2 className="w-3 h-3 animate-spin" />
            ) : (
              <X className="w-3 h-3" />
            )}
          </button>
        )}
      </div>
    );
  },
);

SearchInput.displayName = "SearchInput";
export default SearchInput;

import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useRef,
} from "react";

interface AutoGrowTextareaProps extends React.TextareaHTMLAttributes<HTMLTextAreaElement> {
  /** 最小高度（px）。不传则不强制下界。 */
  minHeight?: number;
  /** 最大高度（px），超过后出现滚动条。默认 160。 */
  maxHeight?: number;
}

// AutoGrowTextarea 根据内容自动调整高度的 textarea：
// 初始按 minHeight（或 rows/CSS）显示，输入增多时高度增长，达到 maxHeight 后停止增长并滚动。
// 复用 ChatInput 的 scrollHeight 模式，抽成通用组件供审批框等多处复用。
const AutoGrowTextarea = forwardRef<HTMLTextAreaElement, AutoGrowTextareaProps>(
  (
    { minHeight, maxHeight = 160, value, onChange, className, ...rest },
    ref,
  ) => {
    const innerRef = useRef<HTMLTextAreaElement>(null);
    useImperativeHandle(ref, () => innerRef.current as HTMLTextAreaElement);

    const adjust = useCallback(() => {
      const el = innerRef.current;
      if (!el) return;
      el.style.height = "auto";
      let h = el.scrollHeight;
      if (maxHeight && h > maxHeight) h = maxHeight;
      if (minHeight && h < minHeight) h = minHeight;
      el.style.height = h + "px";
    }, [minHeight, maxHeight]);

    // 受控 value 变化时调整（程序化设值也能触发）
    useEffect(() => {
      adjust();
    }, [value, adjust]);

    return (
      <textarea
        ref={innerRef}
        value={value}
        onChange={(e) => {
          onChange?.(e);
          adjust();
        }}
        className={className}
        {...rest}
      />
    );
  },
);

AutoGrowTextarea.displayName = "AutoGrowTextarea";

export default AutoGrowTextarea;

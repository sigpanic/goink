import {
  isValidElement,
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useTranslation } from "react-i18next";
import ReactMarkdown from "react-markdown";
import type { Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import remarkMath from "remark-math";
import rehypeKatex from "rehype-katex";
import rehypeRaw from "rehype-raw";
import rehypeSanitize from "rehype-sanitize";
import hljs from "highlight.js/lib/common";
import "katex/dist/katex.min.css";
import "./Markdown.css";

interface MarkdownProps {
  content: string;
  className?: string;
}

interface CodeBlockProps {
  className?: string;
  children?: ReactNode;
}

function getNodeText(children: ReactNode): string {
  if (children === null || children === undefined) {
    return "";
  }

  if (Array.isArray(children)) {
    return children.map(getNodeText).join("");
  }

  if (typeof children === "string" || typeof children === "number") {
    return String(children);
  }

  return "";
}

function CodeBlock({ className, children }: CodeBlockProps) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  const lang = className?.replace(/^language-/, "") || "";
  const code = getNodeText(children).replace(/\n$/, "");
  const highlightedCode = useMemo(() => {
    if (!lang || !hljs.getLanguage(lang)) {
      return null;
    }

    return hljs.highlight(code, { language: lang, ignoreIllegals: true }).value;
  }, [code, lang]);

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(code).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  }, [code]);

  return (
    <div className="markdown-code-block group not-prose">
      <div className="markdown-code-toolbar">
        <span className="markdown-code-lang">{lang || "text"}</span>
        <button
          type="button"
          onClick={handleCopy}
          className="markdown-code-copy"
          aria-label={t("markdown.copyCode")}
        >
          {copied ? t("markdown.copied") : t("markdown.copy")}
        </button>
      </div>
      <pre className="markdown-code-pre">
        {highlightedCode ? (
          <code
            className={className}
            dangerouslySetInnerHTML={{ __html: highlightedCode }}
          />
        ) : (
          <code className={className}>{code}</code>
        )}
      </pre>
    </div>
  );
}

let mermaidModule: typeof import("mermaid").default | null = null;
function loadMermaid() {
  if (!mermaidModule) {
    return import("mermaid").then((m) => {
      mermaidModule = m.default;
      mermaidModule.initialize({
        startOnLoad: false,
        suppressErrorRendering: true,
      });
      return mermaidModule;
    });
  }
  return Promise.resolve(mermaidModule);
}

function MermaidBlock({ code }: { code: string }) {
  const { t } = useTranslation();
  const [svg, setSvg] = useState("");
  const [error, setError] = useState(false);
  const [scale, setScale] = useState(1);
  const containerRef = useRef<HTMLDivElement>(null);
  const isPanning = useRef(false);
  const panStart = useRef({ x: 0, y: 0, scrollX: 0, scrollY: 0 });
  const uid = useId();
  const idRef = useRef(`m-${uid.replace(/:/g, "")}`);

  // 防抖渲染：流式输出时 code 频繁变化，延迟等稳定后再渲染。不清空旧 SVG，避免源码/图表来回切换导致页面抖动。
  useEffect(() => {
    setError(false);

    let cancelled = false;
    const timer = setTimeout(() => {
      if (cancelled) return;
      loadMermaid().then(async (mermaid) => {
        if (cancelled) return;
        const dark = document.documentElement.classList.contains("dark");
        mermaid.initialize({
          startOnLoad: false,
          theme: dark ? "dark" : "default",
          suppressErrorRendering: true,
        });
        try {
          const cleanCode = code.replace(/^---\n[\s\S]*?---\n/, "");
          const { svg: s } = await mermaid.render(idRef.current, cleanCode);
          if (!cancelled) {
            setSvg(s);
            setScale(1);
          }
        } catch {
          if (!cancelled) {
            setError(true);
            setSvg("");
          }
        }
      });
    }, 200);

    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [code]);

  // 非 passive 滚轮监听，阻止页面滚动
  useEffect(() => {
    const el = containerRef.current;
    if (!el || !svg) return;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      setScale((prev) => Math.min(3, Math.max(0.25, prev - e.deltaY * 0.001)));
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
  }, [svg]);

  const handleMouseDown = useCallback(
    (e: React.MouseEvent) => {
      const el = containerRef.current;
      if (!el || scale <= 1) return;
      isPanning.current = true;
      panStart.current = {
        x: e.clientX,
        y: e.clientY,
        scrollX: el.scrollLeft,
        scrollY: el.scrollTop,
      };
      el.style.cursor = "grabbing";
    },
    [scale],
  );

  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      if (!isPanning.current) return;
      const el = containerRef.current;
      if (!el) return;
      el.scrollLeft = panStart.current.scrollX + panStart.current.x - e.clientX;
      el.scrollTop = panStart.current.scrollY + panStart.current.y - e.clientY;
    };
    const onUp = () => {
      isPanning.current = false;
      if (containerRef.current)
        containerRef.current.style.cursor = scale > 1 ? "grab" : "default";
    };
    document.addEventListener("mousemove", onMove);
    document.addEventListener("mouseup", onUp);
    return () => {
      document.removeEventListener("mousemove", onMove);
      document.removeEventListener("mouseup", onUp);
    };
  }, [scale]);

  const zoomIn = useCallback(
    () => setScale((prev) => Math.min(3, prev + 0.25)),
    [],
  );
  const zoomOut = useCallback(
    () => setScale((prev) => Math.max(0.25, prev - 0.25)),
    [],
  );
  const zoomReset = useCallback(() => setScale(1), []);

  return (
    <div className="markdown-code-block not-prose mermaid-host">
      <div className="mermaid-code-toolbar">
        <span className="markdown-code-lang">
          mermaid{error ? " error" : ""}
        </span>
        {svg && (
          <div className="mermaid-zoom-controls">
            <button
              onClick={zoomOut}
              disabled={scale <= 0.25}
              className="mermaid-zoom-btn"
              title={t("markdown.zoomOut")}
            >
              −
            </button>
            <button onClick={zoomReset} className="mermaid-zoom-label">
              {Math.round(scale * 100)}%
            </button>
            <button
              onClick={zoomIn}
              disabled={scale >= 3}
              className="mermaid-zoom-btn"
              title={t("markdown.zoomIn")}
            >
              +
            </button>
          </div>
        )}
      </div>
      {svg ? (
        <div
          ref={containerRef}
          className="mermaid-diagram"
          onMouseDown={handleMouseDown}
          style={{ cursor: scale > 1 ? "grab" : "default" }}
        >
          <div
            className="mermaid-scaled"
            style={{
              transform: `scale(${scale})`,
              transformOrigin: "top left",
            }}
            dangerouslySetInnerHTML={{ __html: svg }}
          />
        </div>
      ) : (
        <pre className="mermaid-code-pre">
          <code>{code}</code>
        </pre>
      )}
    </div>
  );
}

function PreBlock({ children }: { children?: ReactNode }) {
  if (!isValidElement(children)) {
    return <pre className="markdown-code-pre">{children}</pre>;
  }

  const className: string = (children.props as any)?.className || "";

  if (!className) {
    return <pre className="markdown-code-pre">{children}</pre>;
  }

  const lang = className.replace(/^language-/, "");

  if (lang === "mermaid") {
    const code = getNodeText((children.props as any).children).replace(
      /\n$/,
      "",
    );
    return <MermaidBlock code={code} />;
  }

  return (
    <CodeBlock className={className}>
      {(children.props as any).children}
    </CodeBlock>
  );
}

const components: Components = {
  a: ({ href, children }) => (
    <a href={href} target="_blank" rel="noopener noreferrer">
      {children}
    </a>
  ),
  pre: ({ children }) => <PreBlock>{children}</PreBlock>,
  code: ({ className, children }) => (
    <code className={className}>{children}</code>
  ),
  img: ({ src, alt }) => <img src={src} alt={alt || ""} loading="lazy" />,
  input: ({ checked, type, ...props }) => (
    <input type={type} checked={checked} readOnly {...props} />
  ),
  details: ({ children, ...props }) => <details {...props}>{children}</details>,
  summary: ({ children, ...props }) => <summary {...props}>{children}</summary>,
};

export default function Markdown({ content, className }: MarkdownProps) {
  const normalizedContent = content.replace(/\r\n?/g, "\n").replace(/^\n+/, "");

  return (
    <div className={`prose prose-sm max-w-none ${className || ""}`}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm, remarkMath]}
        rehypePlugins={[rehypeKatex, rehypeRaw, rehypeSanitize]}
        components={components}
      >
        {normalizedContent}
      </ReactMarkdown>
    </div>
  );
}

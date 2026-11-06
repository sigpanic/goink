import { Component, useState, type ReactNode } from "react";
import Editor, {
  DiffEditor as BaseDiffEditor,
  type EditorProps,
  type DiffEditorProps,
} from "@monaco-editor/react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { ensureMonaco } from "@/lib/monaco";
import { toErrorMessage } from "@/utils/error";

interface BoundaryProps {
  children: (onReady: () => void) => ReactNode;
  preview: ReactNode;
  width?: string | number;
  height?: string | number;
  t: TFunction;
}

interface BoundaryState {
  phase: "loading" | "mounting" | "ready" | "error";
  error: string;
}

function Loading() {
  const { t } = useTranslation();
  return (
    <div role="status" className="p-4 text-sm text-muted-foreground">
      {t("editor.loading")}
    </div>
  );
}

function Failure({
  error,
  onRetry,
  children,
}: {
  error: string;
  onRetry: () => void;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const [copyStatus, setCopyStatus] = useState("");

  const copyError = async () => {
    try {
      await navigator.clipboard.writeText(error);
      setCopyStatus(t("editor.copied"));
    } catch {
      setCopyStatus(t("editor.copyFailed"));
    }
  };

  return (
    <div className="flex h-full min-h-0 flex-col gap-3 overflow-auto p-4">
      <div role="alert" className="shrink-0 text-sm">
        <p className="font-medium text-destructive">{t("editor.loadFailed")}</p>
        <p className="mt-1 text-muted-foreground">{t("editor.recoveryHint")}</p>
        <pre className="mt-2 max-h-24 select-text overflow-auto whitespace-pre-wrap break-all text-xs">
          {error}
        </pre>
      </div>
      <div className="flex shrink-0 flex-wrap items-center gap-2 text-sm">
        <button
          type="button"
          className="rounded border px-3 py-1 hover:bg-muted"
          onClick={onRetry}
        >
          {t("common.retry")}
        </button>
        <button
          type="button"
          className="rounded border px-3 py-1 hover:bg-muted"
          onClick={() => void copyError()}
        >
          {t("editor.copyError")}
        </button>
        <span role="status" className="text-xs text-muted-foreground">
          {copyStatus}
        </span>
      </div>
      {children}
    </div>
  );
}

class MonacoBoundary extends Component<BoundaryProps, BoundaryState> {
  state: BoundaryState = { phase: "loading", error: "" };
  private generation = 0;
  private timer: ReturnType<typeof setTimeout> | undefined;

  static getDerivedStateFromError(error: unknown): Partial<BoundaryState> {
    return { phase: "error", error: toErrorMessage(error) };
  }

  componentDidMount() {
    this.start();
  }

  componentDidCatch(error: Error) {
    this.generation++;
    this.clearTimer();
    console.error("Monaco editor initialization failed:", error);
  }

  componentWillUnmount() {
    this.generation++;
    this.clearTimer();
  }

  private clearTimer() {
    clearTimeout(this.timer);
    this.timer = undefined;
  }

  private fail(error: unknown, generation: number) {
    if (generation !== this.generation) return;
    this.generation++;
    this.clearTimer();
    this.setState({ phase: "error", error: toErrorMessage(error) });
    console.error("Monaco editor initialization failed:", error);
  }

  private start = () => {
    const generation = ++this.generation;
    this.clearTimer();
    this.setState({ phase: "loading", error: "" });
    // 同时覆盖模块加载和编辑器挂载，避免任一阶段无限等待。
    this.timer = setTimeout(() => {
      this.fail(new Error(this.props.t("editor.loadTimeout")), generation);
    }, 15_000);
    void ensureMonaco()
      .then(() => {
        if (generation === this.generation)
          this.setState({ phase: "mounting" });
      })
      .catch((error: unknown) => this.fail(error, generation));
  };

  render() {
    const { phase, error } = this.state;
    const generation = this.generation;
    return (
      <div
        className="min-h-0 min-w-0"
        style={{
          width: this.props.width ?? "100%",
          height: this.props.height ?? "100%",
        }}
      >
        {phase === "error" ? (
          <Failure error={error} onRetry={this.start}>
            {this.props.preview}
          </Failure>
        ) : phase === "loading" ? (
          <Loading />
        ) : (
          this.props.children(() => {
            if (generation !== this.generation) return;
            this.clearTimer();
            this.setState({ phase: "ready" });
          })
        )}
      </div>
    );
  }
}

function Preview({ value, label }: { value: string; label: string }) {
  return (
    <label className="flex min-h-32 min-w-0 flex-1 flex-col gap-1 text-xs text-muted-foreground">
      {label}
      <textarea
        readOnly
        aria-label={label}
        value={value}
        className="min-h-24 w-full flex-1 resize-none rounded border bg-background p-3 text-sm text-foreground"
      />
    </label>
  );
}

export default function MonacoEditor(props: EditorProps) {
  const { t } = useTranslation();
  return (
    <MonacoBoundary
      t={t}
      width={props.width}
      height={props.height}
      preview={
        <Preview
          value={props.value ?? props.defaultValue ?? ""}
          label={t("editor.readOnlyContent")}
        />
      }
    >
      {(onReady) => (
        <Editor
          {...props}
          width="100%"
          height="100%"
          loading={<Loading />}
          onMount={(editor, monaco) => {
            onReady();
            props.onMount?.(editor, monaco);
          }}
        />
      )}
    </MonacoBoundary>
  );
}

export function DiffEditor(props: DiffEditorProps) {
  const { t } = useTranslation();
  return (
    <MonacoBoundary
      t={t}
      width={props.width}
      height={props.height}
      preview={
        <div className="flex min-h-40 flex-1 gap-3">
          <Preview
            value={props.original ?? ""}
            label={t("editor.originalContent")}
          />
          <Preview
            value={props.modified ?? ""}
            label={t("editor.modifiedContent")}
          />
        </div>
      }
    >
      {(onReady) => (
        <BaseDiffEditor
          {...props}
          width="100%"
          height="100%"
          loading={<Loading />}
          onMount={(editor, monaco) => {
            onReady();
            props.onMount?.(editor, monaco);
          }}
        />
      )}
    </MonacoBoundary>
  );
}

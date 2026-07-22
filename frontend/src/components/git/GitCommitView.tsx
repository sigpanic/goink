import { DiffEditor } from "@monaco-editor/react";
import { FileCode, FileText } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useTheme, type Theme } from "@/hooks/useTheme";
import type { git } from "@/lib/wailsjs/go/models";

const MONACO_THEME: Record<Theme, string> = { light: "light", dark: "vs-dark" };

const DIFF_OPTIONS = {
  minimap: { enabled: false },
  scrollBeyondLastLine: false,
  fontSize: 15,
  lineHeight: 26,
  fontFamily: "'Noto Serif SC', 'Source Han Serif SC', serif",
  lineNumbers: "off",
  wordWrap: "on",
  automaticLayout: true,
  readOnly: true,
  renderSideBySide: false,
  renderIndicators: true,
} as const;

function getLanguage(path: string): string {
  if (path.endsWith(".md")) return "markdown";
  if (path.endsWith(".json")) return "json";
  if (path.endsWith(".yaml") || path.endsWith(".yml")) return "yaml";
  if (path.endsWith(".go")) return "go";
  if (path.endsWith(".ts") || path.endsWith(".tsx")) return "typescript";
  if (path.endsWith(".css")) return "css";
  return "plaintext";
}

interface Props {
  file: git.FileDiff | null;
}

export default function GitCommitView({ file }: Props) {
  const { t } = useTranslation();
  const { theme } = useTheme();

  if (!file) {
    return (
      <main className="flex-1 bg-background flex items-center justify-center border-r">
        <div className="text-center">
          <FileCode className="w-10 h-10 text-muted-foreground/30 mx-auto mb-3" />
          <p className="text-sm text-muted-foreground">
            {t("git.selectFileToDiff")}
          </p>
        </div>
      </main>
    );
  }

  return (
    <main className="flex-1 bg-background flex flex-col min-w-0 min-h-0 border-r overflow-hidden">
      {/* 文件路径头 */}
      <div className="flex items-center gap-2 px-4 py-1.5 border-b shrink-0 bg-muted/10">
        <FileText className="w-3.5 h-3.5 text-muted-foreground" />
        <span className="text-xs text-muted-foreground truncate">
          {file.path}
        </span>
      </div>

      {/* Diff 编辑器 */}
      <div className="flex-1 overflow-hidden">
        <DiffEditor
          height="100%"
          language={getLanguage(file.path)}
          theme={MONACO_THEME[theme]}
          original={file.original}
          modified={file.modified}
          options={DIFF_OPTIONS}
        />
      </div>
    </main>
  );
}

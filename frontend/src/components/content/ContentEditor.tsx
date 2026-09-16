import { useCallback } from "react";
import Editor, { type OnMount } from "@monaco-editor/react";
import { useEditorTabsStore } from "./useEditorTabsStore";

interface Props {
  value: string;
  onChange: (value: string | undefined) => void;
  onMount: OnMount;
  editorTheme?: string;
  // 位置键 `novelId:path:mode`：按 (tab, mode) 独立保存/恢复 Monaco viewState
  // （滚动 + 光标 + 折叠）。不传则不记忆。
  positionKey?: string;
}

type EditorInstance = Parameters<OnMount>[0];
type EditorViewState = Parameters<EditorInstance["restoreViewState"]>[0];

export default function ContentEditor({
  value,
  onChange,
  onMount,
  editorTheme,
  positionKey,
}: Props) {
  const handleMount: OnMount = useCallback(
    (editor, monaco) => {
      if (positionKey) {
        const saved =
          useEditorTabsStore.getState().positions[positionKey]?.viewState;
        if (saved) {
          try {
            editor.restoreViewState(saved as EditorViewState);
          } catch {
            // 保存的 viewState 与当前 model 不兼容时静默忽略（内容已变化）
          }
          // Monaco 失焦时不渲染 caret（光标）。onMount 阶段同步 focus() 可能因
          // 编辑器内部尚未完成焦点绑定而静默失败（wails webview 下尤甚），
          // 延迟到下一帧再 focus。不调用 render(true)：全量重绘会引发视觉闪烁。
          let disposed = false;
          editor.onDidDispose(() => {
            disposed = true;
          });
          requestAnimationFrame(() => {
            if (disposed) return;
            editor.focus();
          });
        }
        // 同步写入 store（内存）——localStorage 落盘由 store 全局 subscribe 防抖负责。
        // 不做组件内 debounce：光标/滚动变化立即进内存，切走不丢；写盘频率不受影响。
        const save = () => {
          const vs = editor.saveViewState();
          if (vs) {
            useEditorTabsStore.getState().setPosition(positionKey, {
              viewState: vs,
              updatedAt: Date.now(),
            });
          }
        };
        editor.onDidScrollChange(save);
        editor.onDidChangeCursorPosition(save);
      }
      onMount(editor, monaco);
    },
    [positionKey, onMount],
  );

  return (
    <Editor
      height="100%"
      language="plaintext"
      theme={editorTheme ?? "light"}
      value={value}
      onChange={onChange}
      onMount={handleMount}
      options={{
        minimap: { enabled: false },
        lineNumbers: "on",
        scrollBeyondLastLine: false,
        fontSize: 17,
        lineHeight: 30,
        fontFamily: "'Noto Serif SC', 'Source Han Serif SC', serif",
        wordWrap: "on",
        automaticLayout: true,
        renderLineHighlight: "line",
        cursorSmoothCaretAnimation: "on",
        smoothScrolling: true,
        mouseWheelZoom: true,
        unicodeHighlight: {
          nonBasicASCII: false,
          ambiguousCharacters: false,
          invisibleCharacters: false,
        },
        suggestOnTriggerCharacters: false,
        quickSuggestions: false,
        wordBasedSuggestions: "off",
      }}
    />
  );
}

import Editor, { type OnMount } from "@monaco-editor/react";

interface Props {
  value: string;
  onChange: (value: string | undefined) => void;
  onMount: OnMount;
  editorTheme?: string;
}

export default function ContentEditor({
  value,
  onChange,
  onMount,
  editorTheme,
}: Props) {
  return (
    <Editor
      height="100%"
      language="plaintext"
      theme={editorTheme ?? "light"}
      value={value}
      onChange={onChange}
      onMount={onMount}
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

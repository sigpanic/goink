import { loader } from "@monaco-editor/react";
import * as monaco from "monaco-editor/esm/vs/editor/editor.api.js";
import "monaco-editor/esm/vs/editor/editor.all.js";
import "monaco-editor/esm/vs/basic-languages/markdown/markdown.contribution.js";
import EditorWorker from "monaco-editor/esm/vs/editor/editor.worker?worker&inline";

// 内联 Worker 避免从 Wails 自定义协议地址加载独立 Worker 脚本。
self.MonacoEnvironment = {
  getWorker() {
    return new EditorWorker();
  },
};

loader.config({ monaco });

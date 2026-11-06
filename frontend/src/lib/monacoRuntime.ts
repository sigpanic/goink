import { loader } from "@monaco-editor/react";
import * as monaco from "monaco-editor";
import EditorWorker from "monaco-editor/esm/vs/editor/editor.worker?worker&inline";
import JsonWorker from "monaco-editor/esm/vs/language/json/json.worker?worker&inline";
import CssWorker from "monaco-editor/esm/vs/language/css/css.worker?worker&inline";
import HtmlWorker from "monaco-editor/esm/vs/language/html/html.worker?worker&inline";
import TypeScriptWorker from "monaco-editor/esm/vs/language/typescript/ts.worker?worker&inline";

// 内联 Worker 避免从 Wails 自定义协议地址加载独立 Worker 脚本。
self.MonacoEnvironment = {
  getWorker(_moduleId, label) {
    switch (label) {
      case "json":
        return new JsonWorker();
      case "css":
      case "scss":
      case "less":
        return new CssWorker();
      case "html":
      case "handlebars":
      case "razor":
        return new HtmlWorker();
      case "typescript":
      case "javascript":
        return new TypeScriptWorker();
      default:
        return new EditorWorker();
    }
  },
};

loader.config({ monaco });

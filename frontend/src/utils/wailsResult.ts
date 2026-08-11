// apperr 新 API 适配层。
//
// 项目存在两套 Wails API：
//   - 旧 API：返回裸数据，失败时 wails reject 字符串（Go err.Error()），前端 try/catch 接住。
//   - 新 API（apperr）：返回 *apperr.Result[T]，始终 HTTP 200，结构 {data, err_code, err_msg}，
//     业务错误体现在 err_code 字段（空字符串表示成功），不 throw。
//
// 详见：
//   - docs/feat/v1.4.0/refactor-plan/04a-query-error-toast.md「apperr 新 API 适配」章节
//   - internal/apperr/apperr.go（Result[T] / Code / CodeFromError）
//
// 问题：新 API 不 throw → queryFn 直接 await 永远不 throw → query 永远不 error →
// 全局错误中间件（queryErrorToast.ts）永远不触发 → 错误静默吞掉（违反规则 8）。
//
// 解决：queryFn 用 unwrapResult(res) 手动解包，err_code 非空时 throw AppErr，
// 把「业务错误」翻译成「query error」，复用现有 query 错误链路（中间件接管 toast）。
// AppErr.errCode 保留供组件 catch 块读取，传给 classifyError 做短码映射具体文案
// （如 SkillMarketplace 的 inline error bar）。

// AppErr 携带后端 apperr.Code，供组件按错误码差异化展示。
// extends Error 保证能被 TanStack Query / 中间件 / try-catch 正常当作 error 处理。
export class AppErr extends Error {
  errCode: string;
  constructor(code: string, msg: string) {
    super(msg);
    this.name = "AppErr";
    this.errCode = code;
  }
}

// unwrapResult 解包 apperr.Result[T]。
// err_code 非空（且非 "ok" 兜底）时 throw AppErr，否则返回 res.data。
// 后端 CodeOK = ""（空字符串），正常成功路径 err_code 为空；为防御性兼顾显式 "ok" 字符串。
// data 可选：失败时后端 data 为零值（wailsjs 绑定生成 data?），成功时由后端保证有值。
export function unwrapResult<T>(res: {
  err_code: string;
  err_msg?: string;
  data?: T;
}): T {
  if (res.err_code && res.err_code !== "ok") {
    throw new AppErr(res.err_code, res.err_msg ?? res.err_code);
  }
  return res.data as T;
}

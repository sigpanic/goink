/**
 * 把 "provider/model" 选择键拆成 [provider, modelID]。
 *
 * 后端 AvailableModel.Key 形如 `name + "/" + m.ID`（internal/llm/config.go），
 * 当 model ID 本身含 `/`（如硅基流动的 `deepseek-ai/DeepSeek-V4-Flash`、
 * OpenRouter 的 `meta-llama/llama-3.1-70b-instruct`）时，整个 key 会变成
 * `siliconflow/deepseek-ai/DeepSeek-V4-Flash`（三段）。直接 `key.split("/")`
 * 只会拿到前两段，把真正的模型名截掉，导致后端 ProviderModel 查不到、
 * 报"模型未找到"。本函数按首个 `/` 拆分，modelID 保留后续所有 `/`，
 * 与后端 lookup 语义一致。
 *
 * @returns [provider, modelID]；无 `/` 时 modelID 为空字符串，调用方可据此前置拦截。
 */
export function splitModelKey(key: string): [string, string] {
  const idx = key.indexOf("/");
  if (idx === -1) return [key, ""];
  return [key.slice(0, idx), key.slice(idx + 1)];
}

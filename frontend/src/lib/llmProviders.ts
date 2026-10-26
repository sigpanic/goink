export const PROVIDER_PROTOCOLS = [
  {
    adapterName: "openai_chat",
    label: "OpenAI Chat",
    providerType: "openai",
    endpoint: "https://api.example.com/v1/chat/completions",
  },
  {
    adapterName: "anthropic_messages",
    label: "Anthropic Messages",
    providerType: "anthropic",
    endpoint: "https://api.example.com/v1/messages",
  },
  {
    adapterName: "openai_responses",
    label: "OpenAI Responses",
    providerType: "openai",
    endpoint: "https://api.example.com/v1/responses",
  },
  {
    adapterName: "gemini_interactions",
    label: "Gemini Interactions",
    providerType: "google",
    endpoint: "https://api.example.com/v1/interactions",
  },
] as const;

export function protocolLabel(adapterName: string): string {
  return (
    PROVIDER_PROTOCOLS.find((p) => p.adapterName === adapterName)?.label ??
    adapterName
  );
}

// 输入名称和协议；同一实例出现在多个模型选项中时，不视为跨协议重名。
export function createProviderLabeler(
  providers: Iterable<readonly [name: string, adapterName: string]>,
): (name: string, adapterName: string) => string {
  const protocols = new Map<string, string>();
  const ambiguous = new Set<string>();
  for (const [name, adapterName] of providers) {
    if (protocols.has(name) && protocols.get(name) !== adapterName) {
      ambiguous.add(name);
    }
    protocols.set(name, adapterName);
  }
  return (name, adapterName) =>
    ambiguous.has(name) ? `${name} (${protocolLabel(adapterName)})` : name;
}

// 与后端 providerViewKey 的 url.PathEscape 保持一致；key 仅用于 UI，不反解析路由。
export function providerViewKey(adapterName: string, name: string): string {
  const escape = (value: string) =>
    encodeURIComponent(value)
      .replace(
        /[!'()*]/g,
        (c) => `%${c.charCodeAt(0).toString(16).toUpperCase()}`,
      )
      .replace(/%(24|26|2B|3A|3D|40)/g, decodeURIComponent);
  return `${escape(adapterName)}/${escape(name)}`;
}

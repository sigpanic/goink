import deepseekRaw from "@/assets/providers/deepseek.svg?raw";
import doubaoRaw from "@/assets/providers/doubao.svg?raw";
import qwenRaw from "@/assets/providers/qwen.svg?raw";
import zhipuRaw from "@/assets/providers/zhipu.svg?raw";
import minimaxRaw from "@/assets/providers/minimax.svg?raw";
import mimoRaw from "@/assets/providers/mimo.svg?raw";
import moonshotRaw from "@/assets/providers/moonshot.svg?raw";

const LOGOS: Record<string, string> = {
  deepseek: deepseekRaw,
  doubao: doubaoRaw,
  qwen: qwenRaw,
  zhipu: zhipuRaw,
  minimax: minimaxRaw,
  mimo: mimoRaw,
  moonshot: moonshotRaw,
};

export default function ProviderIcon({
  provider,
  className,
}: {
  provider: string;
  className?: string;
}) {
  const raw = LOGOS[provider];
  if (!raw) return null;
  return (
    <span className={className} dangerouslySetInnerHTML={{ __html: raw }} />
  );
}

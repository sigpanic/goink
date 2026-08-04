import type { QueryClient } from "@tanstack/react-query";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";
import i18n from "@/i18n";

// queryKey 前缀 → i18n key 映射表。
// 后续领域 query 化时在此补：locations / storyarcs / timeline / reader / preferences /
// novel-settings / style-samples / skills / sessions / chapters 等。
// 漏配时 fallback 用 `${prefix}.loadFailed`，i18n 不存在时再 fallback 到
// `${prefix} load failed`，保证不静默。
const QUERY_ERROR_I18N: Record<string, string> = {
  characters: "character.charsLoadFailed",
  "character-relations": "character.relationsLoadFailed",
  novels: "novel.loadFailed",
  // 后续领域 query 化时补：
  locations: "location.locationsLoadFailed",
  "location-relations": "location.relationsLoadFailed",
  storyarcs: "storyarc.arcsLoadFailed",
  "arc-nodes": "storyarc.nodesLoadFailed",
  "max-chapter": "storyarc.maxChapterLoadFailed",
  timeline: "timeline.loadFailed",
  "chapter-plans": "timeline.chapterPlansLoadFailed",
  reader: "reader.loadFailed",
  preferences: "preference.loadFailed",
  // novel-settings: "novelSetting.loadFailed",
  // "style-samples": "styleSample.loadFailed",
  // skills: "skill.loadFailed",
  // sessions: "session.loadFailed",
  // chapters: "chapter.loadFailed",
};

// 4a 全局 query 错误 toast 中间件。
// 设计依据见 docs/feat/v1.4.0/refactor-plan/04a-query-error-toast.md。
//
// 触发时机：query state 变 error（含 retry 全部失败后、refetch 失败、invalidateQueries 后 refetch 失败）。
// 不触发：retry 期间（state.status='pending'）、refetch 成功（action.type='success'）。
// 不接入：mutation 错误（mutationCache 不被本中间件订阅）；mutation 由各自 onError 处理。
//
// 无需去重：subscribe 是 query 级别 callback，多组件订阅同 queryKey 也只 fire 1 次；
// retry 期间不 fire error action；refetch 失败时 TanStack Query 新建 error 对象引用必变。
//
// 组件卸载静默：observers.length === 0 时 query 无组件订阅（用户已离开该页面），
// 后台 refetch 失败不 toast，避免用户在别的页面突然看到已离开页面的报错。
export function installQueryErrorToast(queryClient: QueryClient): () => void {
  return queryClient.getQueryCache().subscribe((event) => {
    // 只关心 query 状态变 error 的事件
    if (event.type !== "updated") return;
    if (event.action.type !== "error") return;

    const query = event.query;
    const error = query.state.error;
    if (!error) return;

    // 组件卸载静默：无 observer 订阅时，后台 refetch 失败不 toast
    if (query.observers.length === 0) return;

    // 查 i18n key：先按 queryKey 前缀查映射表，没匹配时 fallback 用 `${prefix}.loadFailed`
    const prefix = String(query.queryKey[0] ?? "");
    const i18nKey = QUERY_ERROR_I18N[prefix] ?? `${prefix}.loadFailed`;
    const label = i18n.exists(i18nKey)
      ? i18n.t(i18nKey)
      : `${prefix} load failed`;

    toastError(`${label}: ${toErrorMessage(error)}`);
    console.error(`[query error] ${prefix}:`, error);
  });
}

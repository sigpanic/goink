// queryKey 规范：复数=列表，单数=单个，父级 id 作第二段，子级 id 作第三段。
// 所有 useQuery/mutation 失效用本文件常量，禁止裸写字符串数组。
// 详见 docs/feat/v1.4.0/refactor-plan/00-conventions.md

export const novelKeys = {
  all: ["novels"] as const,
  detail: (id: number) => ["novel", id] as const,
};

export const chapterKeys = {
  list: (novelId: number) => ["chapters", novelId] as const,
  detail: (filePath: string) => ["chapter", filePath] as const,
};

// content: GetContent(novelId, filePath) 读文件内容（章节正文/大纲/goink.md/skill）。
// 与 chapterKeys 区分：chapterKeys 是章节元数据列表（GetChapters），contentKeys 是文件内容缓存。
// 5.2 commit 1：useFileContent 基于 queryClient.fetchQuery 走此 key，多 tab 共享缓存。
export const contentKeys = {
  detail: (novelId: number, filePath: string) =>
    ["content", novelId, filePath] as const,
};

export const characterKeys = {
  list: (novelId: number) => ["characters", novelId] as const,
  detail: (id: number) => ["character", id] as const,
  relations: (novelId: number) => ["character-relations", novelId] as const,
};

export const locationKeys = {
  list: (novelId: number) => ["locations", novelId] as const,
  detail: (id: number) => ["location", id] as const,
  relations: (novelId: number) => ["location-relations", novelId] as const,
};

export const storyarcKeys = {
  list: (novelId: number) => ["storyarcs", novelId] as const,
  detail: (id: number) => ["storyarc", id] as const,
};

// arc-nodes: 后端 GetArcNodes(novelId, fromChapter, toChapter) 第二三参数是章节窗口非 arcId，
// 无按 arcId 拉取的 API，故 queryKey 第二段用 novelId（全量缓存，invalidate 一次刷全部）。
export const arcNodeKeys = {
  list: (novelId: number) => ["arc-nodes", novelId] as const,
  detail: (id: number) => ["arc-node", id] as const,
};

// maxChapter: 小说最大章节号（用于 storyarc 章节窗口中心 windowCenter）。
export const maxChapterKeys = {
  detail: (novelId: number) => ["max-chapter", novelId] as const,
};

export const timelineKeys = {
  list: (novelId: number) => ["timeline", novelId] as const,
  detail: (id: number) => ["timeline-entry", id] as const,
};

// chapter-plans: 章节计划 3-slot（next/near/far），GetChapterPlans(novelId) 独立 API。
export const chapterPlanKeys = {
  list: (novelId: number) => ["chapter-plans", novelId] as const,
};

export const readerKeys = {
  list: (novelId: number) => ["reader", novelId] as const,
  detail: (id: number) => ["reader-perspective", id] as const,
};

export const preferenceKeys = {
  list: (novelId: number) => ["preferences", novelId] as const,
  detail: (id: number) => ["preference", id] as const,
};

export const novelSettingKeys = {
  list: (novelId: number) => ["novel-settings", novelId] as const,
  detail: (id: number) => ["novel-setting", id] as const,
};

export const styleSampleKeys = {
  list: (novelId: number) => ["style-samples", novelId] as const,
  detail: (id: number) => ["style-sample", id] as const,
};

export const skillKeys = {
  all: ["skills"] as const,
  detail: (name: string) => ["skill", name] as const,
};

// chat 领域 GET 端点 query key（5.1 commit 1）。
// models/settings 是全局配置，但首个消费方是 ChatPanel，hook 暂放 components/chat/，
// 后续全局配置领域迁移时共享缓存。
export const modelKeys = {
  all: ["models"] as const,
};

export const settingsKeys = {
  all: ["settings"] as const,
};

export const slashCommandKeys = {
  list: (novelId: number) => ["slash-commands", novelId] as const,
};

export const sessionMessagesKeys = {
  detail: (sessionId: string) => ["session-messages", sessionId] as const,
};

export const sessionKeys = {
  // list: 单页查询（ChatPanel 最近会话 page=1 size=5）。queryKey 含分页/搜索参数，
  // 与 SessionHistory 的 infiniteList 区分缓存（size/search 不同则不共享）。
  list: (
    novelId: number,
    page: number,
    size: number,
    search: string,
  ) => ["sessions", novelId, page, size, search] as const,
  // infiniteList: SessionHistory 无限滚动序列（size=20 + search）。
  // page 由 useInfiniteQuery 的 pageParam 管理，不进 key；search 变化触发新 query。
  infiniteList: (novelId: number, size: number, search: string) =>
    ["sessions", "infinite", novelId, size, search] as const,
  // detail: session_id 是 string（修原 number 类型 bug）。
  detail: (id: string) => ["session", id] as const,
};

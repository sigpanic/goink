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
  // all: 前缀失效用（invalidateQueries 失效 list + infiniteList 全部缓存）。
  all: ["style-samples"] as const,
  // list: StyleView 单页分页（page 进 key，page 变化触发新 query）。
  // 与 infiniteList 区分缓存（StyleSampleList 无限滚动用 infiniteList）。
  list: (
    novelId: number,
    page: number,
    size: number,
    search: string,
  ) => ["style-samples", novelId, page, size, search] as const,
  // infiniteList: StyleSampleList 无限滚动（page 由 pageParam 管理，不进 key）。
  // search 变化触发新 query。对齐 sessionKeys.infiniteList 模式。
  infiniteList: (novelId: number, size: number, search: string) =>
    ["style-samples", "infinite", novelId, size, search] as const,
  detail: (id: number) => ["style-sample", id] as const,
};

export const skillKeys = {
  // all: 前缀失效用（invalidateQueries 失效所有 novel 的 list 缓存）。
  all: ["skills"] as const,
  // list: 与 ListSkillsInput.novel_id 对齐，避免跨 novel 串缓存（不同 novel 的 novel 层 skill 不同）。
  list: (novelId: number) => ["skills", novelId] as const,
  detail: (name: string) => ["skill", name] as const,
  // remoteList: SkillMarketplace 远程技能市场列表（apperr 新 API）。
  // input 含 page/size/query，进 key 避免不同分页/搜索串缓存。
  remoteList: (input: { page: number; size: number; query: string }) =>
    ["remote-skills", input.page, input.size, input.query] as const,
  // remoteContent: SkillMarketplace 远程技能内容（detail phase）。
  remoteContent: (name: string) => ["remote-skill-content", name] as const,
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

// 5.8 commit 1：LLM 配置 query key。
// GetLLMConfig 返回 LLMConfigView（含 providers），与 modelKeys（GetModels 返回 model 列表）语义不同，独立 key。
// useSaveLLMConfig onSuccess invalidate 此 key，让 ModelConfigTab 自动 refetch。
export const llmConfigKeys = {
  all: ["llm-config"] as const,
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

// 5.4 commit 5：git 领域 query key。
// git 提交历史走游标分页（afterHash），不进 key；commitFiles/fileDiff 按 hash+filePath 拉取。
export const gitCommitKeys = {
  // infiniteList: GetCommitLog 游标分页（GitHistoryList size=50）。
  // pageParam 是 afterHash 字符串（git log 天然分页方式），不进 key；
  // size 进 key 以区分不同分页大小的缓存。
  infiniteList: (novelId: number, size: number) =>
    ["git-commits", novelId, size] as const,
};

export const commitFileKeys = {
  // list: GetCommitFileList(novelId, hash)，展开 commit 时按需拉取文件列表。
  list: (novelId: number, hash: string) =>
    ["commit-files", novelId, hash] as const,
};

export const fileDiffKeys = {
  // detail: GetFileDiff(novelId, hash, filePath)，选中文件时按需拉取 diff。
  detail: (novelId: number, hash: string, filePath: string) =>
    ["file-diff", novelId, hash, filePath] as const,
};

// 5.5 commit 1：search 领域 query key。
// SearchAll(novelId, query) 全局跨实体搜索。query 字符串编入 key 驱动 refetch +
// 让 query 内置竞态保护接管（替代旧 reqIdRef 手动竞态保护）。
// staleTime=0（搜索是用户主动期望最新结果的操作）+ gcTime 短（防输入字符堆积 cache）。
export const searchKeys = {
  list: (novelId: number, query: string) =>
    ["search", novelId, query] as const,
};

// 5.7 commit 1：profile 领域 query key。
// writingActivity: GetWritingActivity(months) 过去 N 个月写作日历（绿格子数据）。
// writingStats: GetWritingStats() 全局写作统计（无入参，单例缓存）。
// settings 复用 settingsKeys.all（与 chat 共享缓存，profile 读 user_name/avatar 字段）。
export const writingActivityKeys = {
  detail: (months: number) => ["writing-activity", months] as const,
};

export const writingStatsKeys = {
  all: ["writing-stats"] as const,
};

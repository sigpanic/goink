package timeline

import "time"

// ChapterPlan 是 AI 对章节的规划，按 scope 分为三个层级。
//
// 设计原则：
//   - 表级保证每个小说每个 scope 只有一条记录（(novel_id, scope) 联合唯一约束）。
//     next/near/far 表达的是"当前规划状态"而非"历史规划记录"——AI 修改规划时直接 UPSERT 覆盖。
//   - 一个小说固定 3 行，不会增长。旧版本的计划没有查询价值，不需要追留历史。
//   - 与 Python 的关键差异：Python 将章节计划混入 timeline_entries 表（category=chapter_plan），
//     与其他 category 共用字段和查询逻辑，导致注入策略复杂。Go 独立为 chapter_plans 表，字段精简。
//   - 无 target_chapter：时效性由 scope 自然表达，不需要绑定具体章节号。
//
// 注入策略（第 N 章写作时系统自动拼入，AI 无感知）：
//   - next：全量注入（完整 content）
//   - near：全量注入（完整 content）
//   - far ：全量注入（完整 content）
//     固定 3 行，不需要折索引也不需要截断。
//
// MCP 工具实现参考：
//   - get_timeline：内部查询 WHERE novel_id = ?，返回 {next, near, far} 三个槽位
//   - add_timeline_entry：scope="next"/"near"/"far" 时 UPSERT（ON CONFLICT (novel_id, scope) DO UPDATE）
//   - update_timeline_entry：按 (novel_id, scope) 定位并更新 content
type ChapterPlan struct {
	NovelID int64  `json:"novel_id"`
	Scope   string `json:"scope"`   // "next" | "near" | "far"
	Content string `json:"content"` // 计划内容，自然语言，LLM 填写
}

// TimelineEntry 是伏笔和用户创作意图的追踪条目。
//
// 与 Python 版本的差异（26 字段 → 13 字段）：
//
// 保留的优化：
//  1. category 约束枚举（foreshadowing | user_directive）——系统级分类驱动不同的查询和注入策略，
//     不同于偏好和角色关系的自由文本，这里需要精确的行为区分。
//  2. target_chapter 必填（NOT NULL）作为主排序键，不作为过滤条件。
//     LLM 估算不准确不影响（只排不滤），review agent 每章写完后校准。
//  3. 混合注入：近期窗口全量 + 全局索引一行一条。不依赖 target_chapter 做过滤，
//     只依赖它排序。不准确不会丢数据。
//
// 砍掉的字段及理由：
//   - extra_metadata    → content 和 detail_json 已覆盖扩展需求
//   - related_entry_ids → Python 写入但从未被查询遍历，纯粹占位
//   - tags              → 自由标签可放入 detail_json
//   - arc_id            → Python 从未在任何查询或业务逻辑中使用，等 story_arc 真正实现后再加
//   - sequence          → Python 从未被排序或过滤使用
//   - version           → 编辑历史审计，非 AI 写作核心需求
//   - last_editor       → source 字段已区分 ai/user
//   - original_ai_output → Python 保存但从不回读
//   - resolved_at       → 可通过 resolved_chapter 推导
//   - time_horizon      → target_chapter 已表达时间远近，无需额外字段
//
// 排序规则：target_chapter ASC, importance DESC
//   - 下一章的伏笔排最前，第 300 章的排后面
//   - 同一章的按重要度排列
//   - target_chapter 必填，不存在 NULL 排序问题
//
// 注入策略（第 N 章写作时系统自动拼入，AI 无感知）：
//   - 近期窗口全量：target_chapter 在 [N-3, N+5] 且 status != resolved/abandoned
//     → 完整 title + content + detail_json，按 target_chapter ASC
//   - 全部索引：所有未解决条目，target_chapter ASC, importance DESC，100 条截断
//     → 每行一条：id | category | title | target_chapter | importance | source_chapter
//   - 系统提示词强制要求：索引中如存在与当前章节相关的条目，必须在写作前调用
//     get_timeline 获取完整信息，不得凭索引标题猜测内容
//
// Review Agent 职责（每章写完后自动触发）：
//  1. 故事进展超出预期 → 调整 target_chapter（第 200 章 → 第 250 章）
//  2. 伏笔已被故事自然回收但未标记 → 标记 status=resolved, resolved_chapter=当前章
//  3. 与当前剧情冲突的条目 → 上报主 agent，主 agent 决定修改或通知用户
//  4. 暂时考虑review agent提供意见给主agent，再去更新状态，也就是说创作完，先维护状态，然后启动review，同时review 章节内容和状态维护的正确性
//
// MCP 工具实现参考（3 个工具，和 Python 一致）：
//   - get_timeline：统一返回 chapter_plans + time_entries，内部查两张表，AI 无感知表结构
//   - add_timeline_entry：批量创建 1-6 条，事务写入。内部根据 category 路由到此表
//   - update_timeline_entry：更新单条（title/content/importance/target_chapter/status），
//     标记 resolved 时记录 resolved_chapter
type TimelineEntry struct {
	ID                int64     `gorm:"column:id;primaryKey;autoIncrement"      json:"id"`
	NovelID           int64     `gorm:"column:novel_id;not null;index"          json:"novel_id"`
	Category          string    `gorm:"column:category;not null;index"          json:"category"`               // "foreshadowing" | "user_directive"，约束枚举
	Status            string    `gorm:"column:status;not null;index"            json:"status"`                 // "pending" | "resolved" | "abandoned"
	Title             string    `gorm:"column:title;not null"                   json:"title"`                  // 简短标题
	Content           string    `gorm:"column:content"                          json:"content"`                // 详细描述
	DetailJSON        string    `gorm:"column:detail_json"                      json:"detail_json"`            // JSON，category 相关结构化数据（伏笔类型、提示文本等）
	TargetChapter     int       `gorm:"column:target_chapter;not null"          json:"target_chapter"`         // 预计回收章节号，主排序键，必填。不用于过滤，不准确不影响可见性，这个需要提醒llm完成的时候留下准确的章节号
	Importance        int       `gorm:"column:importance;default:3"             json:"importance"`             // 重要度 1-5，默认 3。同 target_chapter 内的次排序键
	SourceChapter     int       `gorm:"column:source_chapter"                   json:"source_chapter"`         // 在哪章创建/埋下的，创建后不可变
	Source            string    `gorm:"column:source"                           json:"source"`                 // "ai" | "user"，谁创建的
	ResolvedChapter   int       `gorm:"column:resolved_chapter"                 json:"resolved_chapter"`       // 在哪章回收，0 表示未回收
	TargetChapterID   *int64    `gorm:"column:target_chapter_id;index"          json:"target_chapter_id"`      // v1.5.0 新增：chapters.id 外键，commit 1.5 由 TargetChapter 反查填充
	SourceChapterID   *int64    `gorm:"column:source_chapter_id;index"          json:"source_chapter_id"`      // v1.5.0 新增：chapters.id 外键，commit 1.5 由 SourceChapter 反查填充
	ResolvedChapterID *int64    `gorm:"column:resolved_chapter_id;index"        json:"resolved_chapter_id"`    // v1.5.0 新增：chapters.id 外键，commit 1.5 由 ResolvedChapter 反查填充
	CreatedAt         time.Time `gorm:"column:created_at;autoCreateTime"        json:"created_at"`
	UpdatedAt         time.Time `gorm:"column:updated_at;autoUpdateTime"        json:"updated_at"`
}

// TableName 指定 GORM 表名。
func (TimelineEntry) TableName() string { return "time_entries" }

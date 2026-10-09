# 年度创作报告 Commit 路线

> 配套 [annual-report-design.md](./annual-report-design.md)。沿用 v1.6.0 专项路线的格式，只列每个 commit 做什么、验证什么；具体实现以代码为准。

## 前置状态

- writing_log 已记录手动保存和 AI 写入的字数变化，本次复用。
- 新增每日统计、年度查询和正式报告页面尚未实现。
- [annual-report-demo.html](../../../frontend/demo/annual-report-demo.html) 是使用模拟数据的独立视觉参考，不代表正式功能已完成；启动前端开发服务后，可通过 /demo/annual-report-demo.html 预览，也可直接用浏览器打开该文件。
- 已确认统计用于创作回顾与宣传，允许偏差；保持按天累计、失败不影响主流程的简单方案。

## 阶段一：开始积累数据

本阶段完成后即可在正常使用中积累数据，不依赖报告页面。

| # | Commit message | 做什么 | 验证 |
|---|---|---|---|
| 1 | `docs(v1.6.1): define annual report metrics and commit roadmap` | 固化两张每日累计表、创建数、近似时长、对话轮数、工具次数、Token 口径，以及本路线。 | 文档 review、链接和格式检查 |
| 2 | `feat(activity): add daily statistics storage` | 新增轻量统计 Store、activity_daily 和 llm_usage_daily 模型及迁移；持久化一次性统计起点，提供原子增量累加；完成初始化注入和 operation_log 排除，统计写入失败只告警。 | Go build/test/lint；新旧数据库初始化、重复初始化保留起点、并发累加、失败隔离、操作日志排除测试 |
| 3 | `feat(activity): track novel chapter character and location creation` | 在常用手动及 AI 创建入口的成功返回位置累计小说、章节、人物和地点数量；普通更新和恢复路径不接入，删除与回退不扣减历史计数。 | Go build/test/lint；代表性的手动与 AI 创建、失败不主动计数、删除后保留累计数验证 |
| 4 | `feat(activity): count conversation turns and tool executions` | 接入用户对话请求被接受时的轮数计数，以及 Agent 实际开始执行工具时的次数计数；内部模型循环不增加轮数，工具成功或失败均计入。 | Go build/test/lint；一轮多工具、未执行工具、失败执行与统计失败隔离验证 |
| 5 | `feat(activity): accumulate daily model token usage` | 从主对话原始单次 usage 累计输入、输出、总 Token 和可取得的缓存、推理明细；按服务商、模型、用途区分，缺失用量跳过，不增加请求审计。 | Go build/test/lint；多次 usage 累加、模型维度、会话累计字段不重复计入、缺失字段验证 |
| 6 | `feat(app): accept approximate creative activity time` | 增加轻量时长记录 API，将前端提供的活跃秒数归入当天；单次最多计 60 秒，统计失败不影响前端操作；补 API 测试并重新生成 Wails 绑定。 | Go build/test/lint；无效值、单次上限、失败隔离测试；绑定生成检查 |
| 7 | `feat(frontend): estimate active creative time` | 在创作工作区根层挂载一个活动 hook；可见、有焦点且近期有交互时每分钟累计时间，闲置约 5 分钟暂停；清理监听与定时器，不补记暂停时间或退出尾段。 | 前端 build/lint/test；活跃、闲置、失焦、暂停恢复和重复挂载验证 |

## 阶段二：年度汇总与正式页面

可以在阶段一验收后暂停后续开发，让数据先积累。

| # | Commit message | 做什么 | 验证 |
|---|---|---|---|
| 8 | `feat(activity): query yearly creative statistics` | 提供固定日期范围的活动、模型用量汇总与每日序列；复用 writing_log 计算年度新增字数、写作活跃日、最长连续天数、月度趋势和作品字数排名；保留统计起点与覆盖区间。 | Go build/test/lint；年度边界、空数据、每日与总量一致性、连续日期和查询错误测试 |
| 9 | `feat(app): expose annual creative report API` | 增加报告查询 DTO/API，转换内部汇总结果并适配错误；App 保持薄层，不承担聚合编排；重新生成 Wails 绑定。 | Go build/test/lint；年度输入、返回字段与错误适配测试；绑定生成检查 |
| 10 | `feat(frontend): display annual creative report` | 将 demo 中的视觉方向用于正式报告，接入真实年度数据；展示创作成果、近似时长、写作热力图、月度趋势、作品排名、对话与工具次数、Token 和模型分布；支持年份切换、空数据、加载及错误状态，沿用主题变量与 i18n。 | 前端 build/lint/test；年份切换、空数据、查询失败、部分年份及明暗主题验证 |

## 后续可选提交

以下提交不阻塞基础记录或年度页面，按后续需求安排。

| # | Commit message | 做什么 | 验证 |
|---|---|---|---|
| A | `feat(activity): include auxiliary model usage` | 模式、风格提取等辅助调用能通过小改动取得 usage 时接入现有每日表；不为全量覆盖重写调用生命周期。 | Go build/test/lint；辅助用途归属、避免与主对话重复累计验证 |
| B | `feat(frontend): visualize daily token usage` | 使用已有每日用量序列展示 Token 热力图，可切换总量、输入和输出，按模型筛选；标注“已记录 Token 用量”，无需新增表。 | 前端 build/lint/test；每日聚合、指标与模型切换、无记录日期展示验证 |
| C | `feat(frontend): export annual report share cards` | 根据实际报告生成分享卡片预览、文案和图片下载；保留近似统计说明，由用户主动操作。 | 前端 build/lint/test；预览数据与报告一致、导出、取消和失败反馈验证 |

## 顺序约束

1. 每日表、Store 和初始化注入先于任何计数接入；新表的 operation_log 排除随数据基础一起交付。
2. 创建、对话与工具、Token 接入分别 review，避免一份提交同时改动所有业务入口；不为此重构现有业务流程。
3. 时长 API 与生成的 Wails 绑定先于前端计时 hook；hook 不依赖年度报告页面是否打开。
4. 年度查询先于报告 API，报告 API 与绑定先于前端真实数据接入。
5. 可选辅助用量在提交 5 后安排；Token 热力图在年度查询/API 能提供每日用量后安排，不要求等分享导出完成。
6. 每个提交保持单一主题；对应测试随功能提交，不单独安排没有实际变化的“验证提交”。

## 验证与提交约定

- Go 命令从仓库根目录执行；前端命令在 frontend/ 执行。按变更范围完成对应检查，避免无理由重复整套验证。
- 重点验证正常路径、原子累加、失败隔离和前端监听清理。异常漏记、少量重复、分钟级误差属于已接受限制，不新增强事务、去重、可靠队列或崩溃恢复测试要求。
- 表中的 Commit message 仅为 subject 建议。实际提交必须有英文 body，解释当次改动、设计原因和相关近似统计限制，subject 与 body 之间保留空行。
- 每项实现完成后报告实际改动与验证结果，等待用户 review；本路线不构成 add、commit 或其他 Git 写操作的授权。
- 用户授权提交后，若 hook 或检查失败导致新增修复，修复完成后重新等待 review 和提交授权。
- 当前交付范围为设计文档、Commit 路线和临时 demo；路线中的正式功能尚未实现。后续在本文件更新实际进度与 commit，不提前标记完成。

# 卷与章节一体化管理 Commit 路线

> 配套 [volume-chapter-management.md](./volume-chapter-management.md)。本路线只列每个 commit 做什么；现有稳定 ID、分卷模型和迁移不在本次重复改造范围内。

## 前置状态

- `internal/volume` 已具备卷 CRUD、排序与删除前章节检查。
- 章节已使用稳定 ID、按卷/排序键计算实时阅读编号。
- 既有 App 与前端已完成章节引用 ID 化。

## 路线

| # | Commit message | 做什么 | 验证 |
|---|---|---|---|
| 1 | `docs(v1.6.0): define unified volume and chapter management` | 固化单一 Tab、章节操作语义和本路线。 | 文档 review |
| 2 | `feat(app): expose volume management APIs` | 暴露统一的 `PlaceVolume`、卷重命名、删除、查询 API；补 App 层测试，并重新生成 Wails 绑定。 | Go 测试、Wails 绑定生成 |
| 3 | `feat(chapter): add structural management operations` | 增加统一的 `PlaceChapter`：创建或移动章节到目标分组的锚点位置，并提供引用受保护删除；处理正文/大纲/RAG 生命周期并补测试。 | Go 测试 |
| 4 | `feat(frontend): add chapter management workspace` | 新增 ActivityBar 入口和宽屏“章节管理”主区域，按卷及未分卷组只读展示实时编号；保留现有写作侧边栏章节列表及聊天运行状态。 | 前端 build/lint、路由与分组测试 |
| 5 | `feat(frontend): manage volumes in chapter workspace` | 接入卷创建、重命名、删除阻塞提示和基于 `PlaceVolume` 的拖拽排序。 | 前端 build/test |
| 6 | `feat(frontend): manage chapter structure` | 接入 `PlaceChapter` 创建和拖拽编排、删除确认及引用清单；支持卷内和跨卷拖拽到指定位置、卷或未分卷组。 | 前端 build/test、关键交互测试 |
| 7 | `feat(frontend): group writing sidebar by volume` | 写作侧栏按卷分组、卷内百章区间选择；以共用的紧凑表单从顶部或目标组追加章节，创建后打开新章。卷纲文件入口留待后续读写提交。 | 前端 build/lint/test、侧栏导航与创建测试 |
| 8 | `feat(frontend): share chapter creation dialog` | 侧栏与章节管理页共用新建章节弹窗、表单与校验；保留各入口的默认卷和插入规则，移动章节继续使用内嵌表单。 | 前端 build/lint/test、弹窗交互与创建回归测试 |

## 顺序约束

1. 后端 API 与 Wails 绑定必须先于前端 API 调用。
2. 章节删除的引用检查、文件清理和 RAG 清理必须作为同一项结构操作交付，不能让前端先暴露删除入口。
3. 前端先落地单一工作区和分组展示，再逐步接入卷操作、章节操作；卷与章节不新增独立 Tab。
4. 完成后更新 [commit-roadmap.md](./commit-roadmap.md) 的进度状态，避免与本专项路线不一致。

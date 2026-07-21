# golangci-lint 错误审计报告

- **扫描时间**：2026-07-21
- **配置**：`.golangci.yml`（v2 语法），启用 `errcheck` / `govet` / `ineffassign` / `staticcheck` / `unused`
- **排除规则**：`dev_test` / `scripts` / `frontend/node_modules` / `build` 路径；`defer Close/Rollback/Destroy` 的 errcheck；测试文件 errcheck
- **工具版本**：golangci-lint v2.12.2，go 1.25.0
- **命令**：`CGO_ENABLED=1 golangci-lint run --timeout=10m ./...`

---

## 一、汇总

共 **51** 个错误。

### 按 linter 分布

| linter | 数量 |
|---|---|
| errcheck | 34 |
| staticcheck | 12 |
| ineffassign | 3 |
| govet | 1 |
| unused | 0 |

### 按是否真问题分布

| 分类 | 数量 | 说明 |
|---|---|---|
| 真问题（建议修） | 36 | staticcheck 12 + ineffassign 3 + govet 1 + errcheck 20 |
| 可忽略（误报/惯用法） | 7 | GORM AddError 4 + defer os.Remove 2 + base.go Marshal→Unmarshal 1 |
| 存疑（看团队标准） | 7 | GORM Register 6 + location_tools.go:329 |

### 按风险分布

| 风险 | 数量 | 说明 |
|---|---|---|
| 中 | 13 | 数据完整性 / 远端报错定位困难 |
| 低偏中 | 2 | 静默吞错影响调试 |
| 低 | 35 | 风格 / 死代码 / 规范别名 |

---

## 二、详细清单

### 2.1 errcheck — `json.Unmarshal` 未检查（13 个）

共性：MCP `update_*` 工具用"`json.Unmarshal(tc.RawArgs, &entity)` 覆盖 DB 加载的实体后 Save"模式实现部分更新。失败时会持久化"半个旧值 + 半个新值"的混合数据。

#### 真问题（11 个，中风险 10 + 低风险 1）

| 文件:行号 | 上下文 | 风险 | 建议 |
|---|---|---|---|
| [internal/agent/agent.go:614](file:///home/nianhe/projects/todo/internal/agent/agent.go#L614) | `parseArgs` 解析 LLM 工具调用 raw args | 低 | 检查 error 并 `slog.Warn` 记录 raw。工具执行用 rawArgs 不受影响，但需可观测性 |
| [internal/mcp_tools/character_tools.go:259](file:///home/nianhe/projects/todo/internal/mcp_tools/character_tools.go#L259) | UpdateCharacter 部分覆盖 | 中 | `return nil, fmt.Errorf("invalid args: %w", err)` |
| [internal/mcp_tools/character_tools.go:329](file:///home/nianhe/projects/todo/internal/mcp_tools/character_tools.go#L329) | UpdateCharacterRelationship 部分覆盖 | 中 | 同上。关系图 append-only，污染会扩散到历史链 |
| [internal/mcp_tools/location_tools.go:357](file:///home/nianhe/projects/todo/internal/mcp_tools/location_tools.go#L357) | UpdateLocation 部分覆盖 | 中 | 同上。可能写出违反树结构的数据 |
| [internal/mcp_tools/location_tools.go:538](file:///home/nianhe/projects/todo/internal/mcp_tools/location_tools.go#L538) | UpdateLocationRelation 部分覆盖 | 中 | 同上。空间关系边污染影响图遍历 |
| [internal/mcp_tools/reader_perspective_tools.go:259](file:///home/nianhe/projects/todo/internal/mcp_tools/reader_perspective_tools.go#L259) | UpdateReaderPerspective 部分覆盖 | 中 | 同上。影响 suspense/misconception 推理 |
| [internal/mcp_tools/storyarc_tools.go:257](file:///home/nianhe/projects/todo/internal/mcp_tools/storyarc_tools.go#L257) | UpdateStoryArc 部分覆盖 | 中 | 同上。可能写出非法状态机转移 |
| [internal/mcp_tools/storyarc_tools.go:396](file:///home/nianhe/projects/todo/internal/mcp_tools/storyarc_tools.go#L396) | UpdateArcNode 部分覆盖 | 中 | 同上。影响弧线进度追踪 |
| [internal/mcp_tools/timeline_tools.go:244](file:///home/nianhe/projects/todo/internal/mcp_tools/timeline_tools.go#L244) | UpdateTimelineEntry 部分覆盖 | 中 | 同上。影响伏笔追踪与章节计划 |
| [internal/session/types.go:74](file:///home/nianhe/projects/todo/internal/session/types.go#L74) | `ToAPIFormat` 解析 assistant 消息 ExtraMetadata 提取 tool_calls | 中 | 失败时 tool_calls 丢失 → OpenAI API 报 "tool_calls required"，定位困难。至少 `log.Error` |
| [internal/session/types.go:89](file:///home/nianhe/projects/todo/internal/session/types.go#L89) | `ToAPIFormat` 解析 tool 消息 ExtraMetadata 提取 tool_call_id | 中 | 同上。tool 消息丢失 tool_call_id 被 API 拒绝 |

**共性建议**：提取辅助函数 `applyPartialUpdate(rawArgs json.RawMessage, entity any) error` 统一处理，8 个 MCP 调用点复用。

#### 可忽略（1 个）

| 文件:行号 | 理由 |
|---|---|
| [internal/mcp_tools/base.go:308](file:///home/nianhe/projects/todo/internal/mcp_tools/base.go#L308) | 输入 `b` 来自上一行 `json.Marshal(s)`，对 schema 结构体 Marshal 必然产出合法 JSON，Unmarshal 实践中不可能失败。建议加 `_, _ =` 显式标注 |

#### 存疑（1 个）

| 文件:行号 | 理由 |
|---|---|
| [internal/mcp_tools/location_tools.go:329](file:///home/nianhe/projects/todo/internal/mcp_tools/location_tools.go#L329) | 反序列化到 `map[string]any` 仅用于判别 LLM 是否传 `parent_location_id`。RawArgs 已被框架解析过，语法合法。但失败时 `hasParent=false` 会把"传 null 清父节点"误判为"没传字段"，属行为正确性 bug，触发概率极低 |

---

### 2.2 errcheck — `gorm.callback.Register` 未检查（6 个，存疑）

全部在 [internal/storage/operation_log.go:80-95](file:///home/nianhe/projects/todo/internal/storage/operation_log.go#L80)，注册 `RegisterOplogHooks` 的 6 个回调（create/update/delete 的 before/after）。

| 行号 | 回调 |
|---|---|
| :80 | `Create().Before("gorm:before_create").Register("oplog:before_create", ...)` |
| :83 | `Create().After("gorm:after_create").Register("oplog:after_create", ...)` |
| :86 | `Update().Before("gorm:before_update").Register("oplog:before_update", ...)` |
| :89 | `Update().After("gorm:after_update").Register("oplog:after_update", ...)` |
| :92 | `Delete().Before("gorm:before_delete").Register("oplog:before_delete", ...)` |
| :95 | `Delete().After("gorm:after_delete").Register("oplog:after_delete", ...)` |

**判断**：存疑（偏可忽略）
- `Register` 返回的 error 来自 `compile()`，仅在 before/after 出现循环依赖时触发
- 引用的 `gorm:before_create` 等都是 GORM 内建回调，循环依赖概率近乎 0
- GORM v1.31.1 内部已通过 `p.db.Logger.Error` 记录 compile 错误，并非完全静默

**建议**：保持现状可接受。若要严格化，让 `RegisterOplogHooks` 返回 error 由调用方 fail-fast。

---

### 2.3 errcheck — `db.AddError` 未检查（4 个，可忽略）

全部在 [internal/storage/operation_log.go](file:///home/nianhe/projects/todo/internal/storage/operation_log.go)，回调内部写 operation_log 失败时挂错误到 db。

| 行号 | 上下文 |
|---|---|
| :141 | afterCreate UPSERT 命中旧值分支，写 update 日志失败 |
| :147 | afterCreate INSERT 分支，写 create 日志失败 |
| :188 | afterUpdate 旧值与新值不同，写 update 日志失败 |
| :220 | afterDelete 旧值非空，写 delete 日志失败 |

**判断**：可忽略（GORM 惯用法）
- 查 GORM v1.31.1 源码 `gorm.go:394-409`，`AddError` 是副作用型 API，把 err 写入 `db.Error`
- 返回值是 `db.Error` 本身（用于链式访问），并非新错误
- 在回调内部 `db.AddError(err)` 是 GORM 官方推荐写法，errcheck 此处为典型误报

**建议**：在 `.golangci.yml` 为 `db.AddError` 配置 errcheck 白名单，或保持现状。

---

### 2.4 errcheck — `fmt.Sscanf` 未检查（4 个，真问题）

| 文件:行号 | 上下文 | 风险 | 建议 |
|---|---|---|---|
| [internal/agent/display.go:195](file:///home/nianhe/projects/todo/internal/agent/display.go#L195) | `outlines/NNN.md` 路径生成展示标签 | 低 | 失败时标签变"第0章大纲"误导用户。改用 `strconv.Atoi` |
| [internal/mcp_tools/rw_tools.go:535](file:///home/nianhe/projects/todo/internal/mcp_tools/rw_tools.go#L535) | `parseChapterNum` 解析章号 | 低 | 路径已由正则 `\d{3,6}` 校验，但忽略 err 是 silently 错误。改返回 `(int, error)` |
| [internal/mcp_tools/rw_tools.go:549](file:///home/nianhe/projects/todo/internal/mcp_tools/rw_tools.go#L549) | `parseOutlineNum` 解析大纲号 | 低 | 同上 |
| [internal/mcp_tools/location_tools.go:446](file:///home/nianhe/projects/todo/internal/mcp_tools/location_tools.go#L446) | 批量创建关系时 `Sscanf("%d-%d")` 反解 key | 低 | "序列化再反序列化"反模式。建议直接存 `pair{a, b}` 结构进 map |

---

### 2.5 errcheck — `defer os.Remove` 未检查（2 个，可忽略）

| 文件:行号 | 上下文 |
|---|---|
| [internal/git/repo.go:135](file:///home/nianhe/projects/todo/internal/git/repo.go#L135) | `DiffContent` 临时空文件清理 |
| [internal/git/repo.go:143](file:///home/nianhe/projects/todo/internal/git/repo.go#L143) | `DiffContent` 第二个临时文件清理 |

**判断**：可忽略（Go 社区惯例）
- defer 中的临时文件清理失败本就无需处理（文件可能已被移走、/tmp 被清理等）

**建议**：若要消除告警，写 `defer func() { _ = os.Remove(tmp.Name()) }()`；不修也合理。

---

### 2.6 errcheck — `w.Write` 未检查（2 个，真问题）

| 文件:行号 | 上下文 | 风险 | 建议 |
|---|---|---|---|
| [internal/logger/logger.go:65](file:///home/nianhe/projects/todo/internal/logger/logger.go#L65) | `fanWriter.Write` 扇出写入多个 writer | 低 | 注释已说明刻意忽略，但后续 writer 失败完全无日志。改 `if _, err := w.Write(p); err != nil { /* slog 到 stderr */ }` 或 `_ =` 标注 |
| [internal/web/fetch.go:195](file:///home/nianhe/projects/todo/internal/web/fetch.go#L195) | `compressionRatio` 写入 `gzip.NewWriter(&buf)` | 低 | 底层 `bytes.Buffer` 永不报错，但 `w.Close()` 同样未检查——Close 才是 gzip 真正可能 flush 失败的地方。检查 Write 和 Close |

---

### 2.7 errcheck — 其他（3 个，真问题）

| 文件:行号 | 上下文 | 风险 | 建议 |
|---|---|---|---|
| [internal/rag/refresh_queue.go:197](file:///home/nianhe/projects/todo/internal/rag/refresh_queue.go#L197) | `RebuildNovel` 先 `DeleteNovel`（DROP TABLE）再 `IndexChunks` | **中** | **数据完整性风险**：DeleteNovel 失败时旧向量未清除，IndexChunks 通过 `CREATE TABLE IF NOT EXISTS` 跳过建表，向残留旧表追加新向量 → 重复向量污染检索。`return fmt.Errorf("rag: rebuild: delete old vectors: %w", err)` |
| [internal/skill/store.go:61](file:///home/nianhe/projects/todo/internal/skill/store.go#L61) | `ListMeta` 调用前 `ReloadUser` | 低偏中 | 失败时用户看到旧 skill 列表且无任何日志。`if err := ...; err != nil { s.logger.Warn(...) }` |
| [internal/skill/store.go:62](file:///home/nianhe/projects/todo/internal/skill/store.go#L62) | `ReloadNovel` | 低偏中 | 同上 |

---

### 2.8 staticcheck（12 个，全部真问题，低风险）

| 文件:行号 | 检查 | 上下文 | 建议 |
|---|---|---|---|
| [internal/githubapi/client.go:158](file:///home/nianhe/projects/todo/internal/githubapi/client.go#L158) | S1008 | `isNetworkError` 的 `if errors.As(...) { return true }; return false` | 改为 `return errors.As(err, &netErr)` |
| [internal/githubapi/client.go:169](file:///home/nianhe/projects/todo/internal/githubapi/client.go#L169) | QF1002 | `classifyStatus` 的 `switch { case status == ... }` | 改为 `switch status { case http.StatusNotFound: ... }` |
| [internal/import/txt.go:215](file:///home/nianhe/projects/todo/internal/import/txt.go#L215) | S1021 | `var chapters []Chapter` 紧跟赋值 | 合并为 `chapters := splitByPositions(...)` |
| [internal/import/txt_test.go:310](file:///home/nianhe/projects/todo/internal/import/txt_test.go#L310) | QF1012 | `sb.WriteString(fmt.Sprintf(...))` | 改为 `fmt.Fprintf(&sb, ...)` |
| [internal/import/txt_test.go:344](file:///home/nianhe/projects/todo/internal/import/txt_test.go#L344) | QF1012 | 同上 | 同上 |
| [internal/mcp_tools/rw_tools.go:279](file:///home/nianhe/projects/todo/internal/mcp_tools/rw_tools.go#L279) | QF1012 | diff 上下文渲染 | 改为 `fmt.Fprintf(&b, "%d\|%s\n", i+1, lines[i])` |
| [internal/mcp_tools/storyarc_tools.go:64](file:///home/nianhe/projects/todo/internal/mcp_tools/storyarc_tools.go#L64) | QF1003 | `if arc.Status == "active" { ... } else if ...` | 改为 `switch arc.Status { case "active": ... }` |
| [internal/pattern/extract_test.go:556](file:///home/nianhe/projects/todo/internal/pattern/extract_test.go#L556) | SA1012 | `e.Extract(nil, ...)` 测试 nil Chapters 早期校验 | 改为 `context.TODO()`。当前 `Extract` 早期返回不会 panic，但传 nil ctx 是反模式 |
| [internal/pattern/extract_test.go:572](file:///home/nianhe/projects/todo/internal/pattern/extract_test.go#L572) | SA1012 | 测试 nil LLMClient 早期校验 | 同上 |
| [internal/pattern/extract_test.go:586](file:///home/nianhe/projects/todo/internal/pattern/extract_test.go#L586) | SA1012 | 测试 NovelID<=0 早期校验 | 同上 |
| [internal/pattern/extract_test.go:600](file:///home/nianhe/projects/todo/internal/pattern/extract_test.go#L600) | SA1012 | 测试空 ProviderName 早期校验 | 同上 |
| [internal/search/service.go:128](file:///home/nianhe/projects/todo/internal/search/service.go#L128) | QF1003 | `if e.Category == "foreshadowing" { subtitle = "伏笔" } else if ...` | 改为 `switch e.Category { ... }`，保留 `subtitle := e.Category` 默认值 |

**说明**：S/QF 类是纯风格优化，行为完全等价；SA1012 是潜在问题（当前不 panic，但未来校验顺序调整可能暴露）。

---

### 2.9 ineffassign（4 个，全部真问题，低风险）

| 文件:行号 | 变量 | 上下文 | 建议 |
|---|---|---|---|
| [internal/agent/agent.go:391](file:///home/nianhe/projects/todo/internal/agent/agent.go#L391) | `interrupted` | `tool_args_invalid` 重试退避 `select` 的 `ctx.Done()` 分支，赋值后 392 行立即 return | 删除该行死代码。return 已由 `ctx.Err()` 表达中断语义 |
| [internal/agent/agent.go:441](file:///home/nianhe/projects/todo/internal/agent/agent.go#L441) | `interrupted` | P2 可恢复错误重试退避 `select` 的 `ctx.Done()` 分支，赋值后 442 行立即 return | 同一死代码模式，删除该行 |
| [internal/import/txt.go:153](file:///home/nianhe/projects/todo/internal/import/txt.go#L153) | `bestCount` | 章节分割模式选择，赋值后从未被读取（后续只读 `bestIdx`） | 删除该行，只保留 `bestIdx = r.idx` |
| [internal/search/service_bench_test.go:67](file:///home/nianhe/projects/todo/internal/search/service_bench_test.go#L67) | `contentPerChapter` | bench setup，67 行赋值被 71 行覆盖前未读取 | 删除 71 行冗余赋值；87 行的 `_ = len(contentPerChapter)` 也应删除（80 行已读取，并非 unused） |

---

### 2.10 govet（1 个，真问题，低风险）

| 文件:行号 | 检查 | 上下文 | 建议 |
|---|---|---|---|
| [internal/storage/operation_log.go:304](file:///home/nianhe/projects/todo/internal/storage/operation_log.go#L304) | inline | `if destValue.Kind() == reflect.Ptr` | 改为 `reflect.Pointer`（Go 1.18+ 规范别名，`Ptr` 未废弃但官方推荐 `Pointer`）。一行替换，零风险 |

---

## 三、修复优先级

### P0 必修（数据完整性，1 个）

- [internal/rag/refresh_queue.go:197](file:///home/nianhe/projects/todo/internal/rag/refresh_queue.go#L197) — `DeleteNovel` 失败导致重复向量污染检索

### P1 建议修（部分覆盖污染 + 调试困难，12 个）

- 8 个 MCP `update_*` 工具的 `json.Unmarshal` 部分覆盖模式（[character_tools.go:259,329](file:///home/nianhe/projects/todo/internal/mcp_tools/character_tools.go#L259)、[location_tools.go:357,538](file:///home/nianhe/projects/todo/internal/mcp_tools/location_tools.go#L357)、[reader_perspective_tools.go:259](file:///home/nianhe/projects/todo/internal/mcp_tools/reader_perspective_tools.go#L259)、[storyarc_tools.go:257,396](file:///home/nianhe/projects/todo/internal/mcp_tools/storyarc_tools.go#L257)、[timeline_tools.go:244](file:///home/nianhe/projects/todo/internal/mcp_tools/timeline_tools.go#L244)）—— 建议提取 `applyPartialUpdate` 辅助函数统一处理
- [session/types.go:74,89](file:///home/nianhe/projects/todo/internal/session/types.go#L74) — ExtraMetadata 损坏导致 API 报错定位困难
- [skill/store.go:61,62](file:///home/nianhe/projects/todo/internal/skill/store.go#L61) — 静默吞错影响调试

### P2 批量清理（风格 / 死代码 / 规范别名，24 个）

- 4 个 `fmt.Sscanf` 未检查
- 2 个 `w.Write` 未检查
- 4 个 ineffassign 死代码（agent.go:391、agent.go:441、txt.go:153、service_bench_test.go:67）
- 1 个 govet `reflect.Ptr` → `reflect.Pointer`
- 12 个 staticcheck（S/QF/SA 类，全部低风险机械化修复）
- 1 个 `agent.go:614` json.Unmarshal（加 `slog.Warn` 记录畸形 JSON）

### 可忽略 / 配置白名单（7 个）

- 4 个 `db.AddError`（GORM 惯用法）—— 建议 `.golangci.yml` 加 errcheck 白名单
- 2 个 `defer os.Remove`（临时文件清理惯用法）—— 建议 `_ =` 标注或白名单
- 1 个 [base.go:308](file:///home/nianhe/projects/todo/internal/mcp_tools/base.go#L308) Marshal→Unmarshal 往返 —— 建议 `_, _ =` 标注

### 存疑（7 个，看团队标准）

- 6 个 `gorm.callback.Register` —— GORM 内部已 logger 记录 compile 错误，引用内建回调循环依赖概率近乎 0。若要严格化，让 `RegisterOplogHooks` 返回 error
- 1 个 [location_tools.go:329](file:///home/nianhe/projects/todo/internal/mcp_tools/location_tools.go#L329) —— 反序列化到 map 用于判别字段存在性，触发概率极低但有行为正确性隐患

---

## 四、配置优化建议

修复 P0/P1 后，可在 `.golangci.yml` 的 `exclusions.rules` 增加白名单（消除剩余误报）：

```yaml
# GORM 惯用法：db.AddError 是副作用型 API，返回值非新错误
- linters: [errcheck]
  text: "Error return value of `.+\\.AddError` is not checked"
# 临时文件清理惯用法
- linters: [errcheck]
  source: "defer .+os\\.Remove"
```

或保持现状，在代码中用 `_ =` 显式标注忽略并配注释说明原因（更显式，但增加代码噪音）。

---

## 五、备注

- `internal/agent/agent.go` 有 3 个错误（:391 :441 ineffassign、:614 errcheck），均为真问题低风险，已纳入修复范围。两个 ineffassign 是同一种死代码模式（重试退避 `select` 的 `ctx.Done()` 分支赋值后立即 return），删除该行即可；errcheck 建议加 `slog.Warn` 记录畸形 JSON（工具执行用 rawArgs 不受影响）。
- 本次审计由 general_purpose_task 子 agent 并行只读分析，未修改任何业务代码。

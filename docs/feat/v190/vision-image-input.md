# v1.8.0 图片输入与 read 主动读图设计

> 2026-10-31。功能提前至 v1.8.0，文档沿用原路径。第一阶段已提交附件 Store 与消息引用映射；第二阶段实现四套协议的 Base64 图片请求、视觉能力与预算校验，待 review。界面和 read 工具接入仍待后续阶段完成。本文件不构成 Git 提交授权。

## 1. 目标与范围

让支持视觉的模型通过两种入口看到图片：用户在聊天中添加图片；模型调用现有 `read` 工具，按需读取已经导入 Goink 的图片。两种入口复用本地附件存储、缩放压缩、消息引用与协议转换能力。

首版包含：

- PNG、JPEG 静态图片；文件选择与粘贴截图；多图、图文混合和纯图片消息。
- 发送前基础缩放与编码压缩，提供保留原始清晰度的选择。
- 本地持久化附件，重启、切换会话、后续追问、重试和上下文压缩时恢复引用。
- OpenAI Chat、OpenAI Responses、Anthropic Messages、Gemini Interactions 四套现有协议。
- `read` 通过 `attachments/<id>` 虚拟路径读取当前小说内的图片，返回模型实际能消费的图片内容。
- 模型能力校验、附件缺失提示与基本请求预算限制。

后续再考虑拖拽、其他图片格式、自动裁图或分块、图床及厂商 Files API、附件自动回收、精细的图片上下文淘汰策略。本期不增加图片生成、视频、PDF 或 OCR 服务；文字截图直接交给视觉模型理解。

## 2. 当前代码基础与缺口

| 位置 | 当前行为 | 需要扩展 |
| --- | --- | --- |
| `internal/llm/adapter/types.go` | `ModelInfo.SupportsVision` 已描述图片输入能力 | 复用能力标记，在消费图片前校验 |
| `internal/llm/adapter/message.go` | `Message.Content` 是字符串，工具结果也使用此字段 | 增加协议无关的图片引用，适用于 user 和 tool |
| `internal/session/llm_message.go` | 在消息与 metadata 之间映射公共模型字段 | 映射图片引用，保留未知 metadata 和既有时间前缀语义 |
| `app/chat.go` | `ChatInput.Message` 只接收文本，用户消息直接写入 DB | 接收附件引用，并在产生本轮记录前完成必要校验 |
| `internal/agent/agent.go`、`safety.go` | 工具结果通过 `resultJSON()` 变成 `tool.Content` 字符串 | 单独传递图片引用，不能把图片数据压进结果 JSON |
| `internal/agent/compress.go` | 保留近期消息副本，通过 session 映射写入新版本 | 保留图片引用与工具关联，复制时不重复存图片 |
| `internal/mcp_tools/rw_tools.go` | `read` 读取指定范围内的文本，支持行号和行范围 | 在文本分支前识别附件虚拟路径 |
| 四套协议的消息构造函数 | 用户输入与普通工具结果目前按文本构造 | 构造原生图片块，处理工具图片结果的协议差异 |
| `ChatInput.tsx`、`ChatPanel.tsx`、`types.ts` | 输入、发送和 `Turn.userMessage` 为文本 | 维护附件草稿、发送引用、重建历史附件 |
| `ChatTurn.tsx`、`MessageBubble.tsx` | 用户气泡依赖非空正文 | 显示图片，支持正文为空的用户消息 |
| `main.go` | AssetServer 已处理封面和头像 | 增加通过附件标识读取预览的受限入口 |

Gemini 当前使用 Interactions，本方案不沿用旧设计文档中的 GenerateContent / `inlineData` 请求格式。

## 3. 数据流与分层

```text
文件选择 / 粘贴截图
        ↓
App：接收导入请求
        ↓
attachment.Store：校验、导入、生成发送版本
        ↓
本地文件 + 附件描述
        ↓
用户消息保存图片引用 ──────────────┐
                                 │
read("attachments/<id>")          │
        ↓                        │
工具结果保存同一种图片引用 ────────┤
                                 ↓
                       session → llm.Message
                                 ↓
                    出站准备：解析引用、读取图片
                                 ↓
                       目标适配器构造图片块
                                 ↓
                         原有模型 HTTP 请求
```

- 新增 `internal/attachment/`，集中管理附件身份、文件读取与图像处理。App 仅做 DTO 转换、调用和错误适配。
- 消息绑定涉及 DB 与附件文件的协调，由内部 Service 编排；session Store 继续只负责 DB 读写，不在事务中压缩图片。
- 图片描述类型按语义复用；图片引用、包含预览信息的附件描述、出站解析后的字节数据分别承担不同职责，避免为每层复制相同字段。
- 由调用方在 `CallOptions.NovelID` 中携带小说作用域；Store 持有 `config.AttachmentsDirPath()` 返回的附件根目录，`Import / Get / Read` 显式接收 `novelID`，由 Store 定位小说子目录，不从模型文本或前端传入的绝对路径确定读取范围。沿用第一阶段已通过的接口与职责。
- 统一出站准备覆盖流式对话、非流式摘要等入口；具体适配器只消费已经解析的图片数据，不自行查询 DB 或推导数据目录。
- 附件包和领域 Service 不反向依赖 App / Wails，也不改动现有 ONNX、向量库或 RefreshQueue singleton。

## 4. 本地存储与消息引用

### 4.1 文件布局

建议放在 `config.DataDirPath()` 下独立的附件目录，与小说 Git 仓库分开：

```text
<DataDir>/attachments/<novel_id>/<attachment_id>/
    manifest.json
    original.png 或 original.jpg
    standard.png 或 standard.jpg
```

附件 ID 由后端生成。manifest 保存归属小说、显示名称、实际 MIME、原始及发送版本的尺寸和字节数、处理策略版本。消息引用中的版本名对应不可变文件；处理参数升级后产生新版本，不能覆盖历史消息正在引用的文件。

导入时复制图片，不依赖用户原文件长期存在。原图保留用于明确选择原始清晰度的场景；默认发送处理后的版本。若原图已经满足标准要求，可由多个逻辑版本引用同一份文件，避免无意义的重复存储。

首版不新增附件数据库表；附件描述由 manifest 管理，消息通过已有 metadata 保存引用。后续确需复杂查询、去重或回收时，再评估独立表和迁移。

### 4.2 消息形状

保留现有 `Content` 作为文本正文，新增可选图片引用列表，不把全部文本消息重写成通用多模态 AST。首版顺序约定为正文在前、图片按用户添加顺序排列。

metadata 示例：

```json
{
  "images": [
    { "attachment_id": "generated-id", "variant": "standard-v1" }
  ]
}
```

工具结果使用相同的 `images` 引用，并继续保存 `tool_call_id` / `tool_name`。尺寸、MIME 与实际读取文件以附件描述为准，不能信任前端自行填写的值。

- `ToLLMMessage`、`ToLLMRequestMessage`、`WithLLMMessage` 和 `Clone` 必须完整保留图片列表。
- 旧消息缺少 `images` 时按纯文本处理，无需回填或扫描历史数据。
- 只允许新消息携带附件，不回写已有 append-only 消息；不改变 API、前端和完整审计查询路径。
- Base64 仅在请求构造阶段生成；DB、草稿 localStorage 和前端事件保存引用及必要展示信息。
- 导入先在临时位置完成验证和处理，再发布最终附件目录；消息写入失败时不留下半成品记录。已经导入但未发送的完整附件可以暂留。
- 首版不自动删除附件。会话删除、回退和压缩都不能依据当前可见消息数立即删文件；未来回收需要检查历史版本、工具结果和草稿引用。已知代价是暂留未引用文件。

## 5. 缩放、编码压缩与 token 预算

文件字节大小与图片 token 是两类预算：编码压缩主要降低磁盘占用和传输量；图片 token 取决于目标模型处理后的尺寸和细节策略，缩小文件到相同像素尺寸不保证减少 token。服务端也可能先缩放，因此本地缩小分辨率不保证按像素比例降低费用。[OpenAI 官方说明](https://developers.openai.com/api/docs/guides/images-vision#calculating-costs)

### 5.1 首版处理策略

- 默认标准版本等比例缩小，建议长边上限 2048px，小图不放大，不裁剪内容。
- JPEG 使用适中质量，建议初始值 85；PNG 保留透明度，优先保留文字和线条清晰度，不统一强转 JPEG。
- 先处理照片方向信息，再确定输出尺寸；避免预览和模型看到不同方向的图片。
- 文字截图、长图支持选择保留原始清晰度。该选择跳过默认缩小，但仍受资源限制和目标服务限制约束，不承诺任意超大图片都能发送。
- 图片处理在导入时完成并缓存；历史追问复用确定的版本，不重复有损编码。
- 解码前检查实际文件类型、尺寸和像素总量；扩展名不能代替内容识别，限制必须在后端执行。

下面是待实现验证的产品初始值，不是厂商 API 上限：

| 限制 | 建议初始值 |
| --- | --- |
| 单张导入原文件 | 20 MiB |
| 单张解码像素总量 | 4000 万像素 |
| 每条用户消息 | 最多 4 张 |
| 单次请求图片文件合计 | 8 MiB，按实际发送版本计算 |

实际请求还包含 Base64 编码开销、文本和历史图片，校验不能只看本轮新增图片。原始清晰度超预算时提示用户改用标准版本或减少图片，不擅自再次压缩。

### 5.2 模型预算与压缩

- 不对 Base64 运行文本 tokenizer。本期按图片尺寸采用统一粗估：`ceil(width / 32) × ceil(height / 32) × 2`，接入现有预算检查。该数值不是厂商计费公式，也不是所有模型用量的严格上界；实际用量继续取服务商 usage，精确的模型差异留待实测后补充。
- 模型支持的 detail / resolution 参数由适配器映射，不假定所有协议支持相同选项，也不把 `low` 当成跨模型固定的费用保证。
- 本期沿用现有消息保留规则；被保留消息继续携带原图片引用，超出保留范围的消息由摘要承接。模型能否继续检查原图与摘要是否记录图片内容是不同能力。
- 摘要生成也需要解析图片并校验视觉能力及输入预算；原始审计消息和本地图片保留。
- 图片估算必须进入现有自动压缩触发预算。若压缩后保留的图片仍超出请求限制，应明确失败，不能反复压缩或静默丢图。
- 后续请求仍携带的历史图片会继续参与输入预算；本期不承诺仅首轮产生图片成本。

## 6. 发给模型的协议转换

本地图片读取后，在现有 JSON 请求里构造图片内容块，无需先建图床。绝对文件路径和前端 `blob:` URL 不能作为远程模型可访问的图片地址。

| 协议 | 用户图片输入 | read 返回图片 |
| --- | --- | --- |
| OpenAI Chat | `content` 数组中的 `image_url`，值为 Base64 Data URL | `tool` 保持文本结果，工具组结果收齐后补一条包含图片的 `user` 消息 |
| OpenAI Responses | `input` 消息中的 `input_image` | `function_call_output.output` 使用文本和图片内容数组 |
| Anthropic Messages | user content 中的 `image`，source 使用 Base64 | 在对应 `tool_result.content` 中放文本和图片块 |
| Gemini Interactions | user input step 中的原生 image content | 在对应 `function_result.result` 中放文本和图片内容，保留 `call_id` |

依据：[OpenAI 图片输入](https://developers.openai.com/api/docs/guides/images-vision)、[OpenAI 工具结果](https://developers.openai.com/api/docs/guides/function-calling#formatting-results)、[Anthropic 工具结果](https://platform.claude.com/docs/en/agents-and-tools/tool-use/handle-tool-calls)、[Gemini Interactions v1](https://ai.google.dev/api/interactions-api-v1)。具体 SDK 字段以仓库锁定版本的序列化结果验证，不能仅凭文档示例判断已支持。

OpenAI Chat 的 tool 消息只接受文本，不能把图片数组直接塞入 tool content。[官方接口定义](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)

### 6.1 OpenAI Chat 出站示例

以下仅展示用户消息片段，省略外层模型配置和其他历史：

```json
{
  "role": "user",
  "content": [
    { "type": "text", "text": "根据这张图描写角色外貌" },
    {
      "type": "image_url",
      "image_url": {
        "url": "data:image/png;base64,<实际图片数据>"
      }
    }
  ]
}
```

工具图片的兼容投影只发生在 OpenAI Chat 适配器的出站副本：

1. 保留原 assistant 工具声明与每个 call 对应的 tool 文本结果。
2. 完整输出该组 tool 结果后，再追加一条 user 图片消息，文字注明来源工具、调用 ID 与附件路径，多张图片按结果顺序组织。
3. 原始数据库仍记录 tool 消息和图片引用，不把兼容消息存成用户真实发言；前端也不创建额外用户 turn。
4. 不修改既有调用去重、孤立结果和原生推理状态兼容规则。不能把图片插在尚未返回完整结果的工具组中间。

四套协议均需覆盖纯图片输入：不能继续以 `Content == ""` 为条件直接跳过消息。Stream 和 Generate 共用转换逻辑，不改变模型响应仍以文本、思考和工具事件展示的方式。

## 7. read 主动读图

调用示例：

```json
{ "path": "attachments/generated-id" }
```

- 在原有文本读取前增加独立的附件路径分支，不把二进制图片交给行号渲染器，也不放宽 edit 的文本路径范围。
- 首版只读取当前小说下已经导入的附件。任意本机绝对路径、远程 URL 和扫描磁盘不属于本期范围。
- 用户图片消息向模型提供稳定的虚拟路径标识；后续模型可引用该标识调用 read。本期不增加独立的附件搜索或列举工具。
- `ToolResult` 增加类型明确的图片引用；`Data` 仍放成功说明、文件名、尺寸等文本信息。Agent 将二者分别写入消息，`resultJSON()` 不序列化图片字节。
- 图片读取默认使用该附件的标准版本；支持通过可选参数明确请求原始清晰度，原有文本参数语义保持不变。图片使用文本行范围参数时返回明确的业务错误。
- 路径或附件不存在、越过小说作用域、模型不能消费图片等是明确的业务失败；磁盘读取等意外依赖错误沿用 Registry 的统一处理。
- 主 Agent 和子 Agent 继续受 `AllowedTools` 限制。子 Agent 若能调用 read，应使用自身模型能力和小说作用域，不继承错误的主模型判断。
- 工具卡片展示读取路径、尺寸及可选缩略图，历史恢复从持久化引用重建，避免依赖只存在于实时事件中的图片数据。

read 不额外调用另一个视觉模型生成描述。执行工具后，由当前视觉模型在下一次推理中直接读取图片。

## 8. 聊天交互与失败处理

- `ChatInput` 支持附件选择、粘贴截图、缩略图、移除和清晰度选择；正文为空且存在有效附件时允许发送。
- 导入进行中或失败的附件不能直接发送。发送失败时保留正文和有效附件，用户可移除问题项后重试。
- 新会话尚无 session ID 时按当前小说导入；切换小说和会话时隔离附件草稿。localStorage 只保存小型引用，不保存 Base64。
- `ChatPanel` 的乐观用户消息、实际请求和 `rebuildTurns` 的历史消息使用相同附件描述。`ChatTurn` 不再只依赖 `userMessage` 非空来显示用户气泡。
- 预览通过后端附件读取入口获取；后端按 ID、归属和版本解析文件，不暴露任意磁盘读取能力。
- 本轮或活跃历史含图片，而目标模型 `SupportsVision=false` 时，在网络请求前明确拒绝并提示切换视觉模型；不静默删图，也不自动发给另一家服务商。
- 附件缺失或损坏时，前端显示占位和原因；需要它的模型请求明确报错。会话历史仍可打开，文本记录仍可查看。
- 新会话标题生成不为标题再发送原图；无正文时使用简短的图片会话兜底标题。
- 现有 `GOINK_DEBUG=1` 会落盘 HTTP 正文，Base64 图片也会随请求被捕获。联调使用测试图片，沿用现有权限保护；本功能不擅自删除或改变调试日志行为。

## 9. 文件范围与 4 个 commit 路线

下列为实现拆分，不包含本文档本身的提交；不要求中间 commit 可编译，不为通过编译引入临时 API 或兼容层，迁移完成后补齐整体验证。实现与任何 Git 写操作分别遵守仓库授权规则。

| 阶段 | 建议 subject | 主要文件范围与交付 |
| --- | --- | --- |
| 1. 附件与消息基础 | `feat(attachments): persist and preprocess chat images` | 新增 `internal/attachment/`；按需扩展 `internal/config/`；修改 `internal/llm/adapter/message.go`、`internal/llm/types.go`、`internal/session/llm_message.go` 及测试。实现文件导入、标准版本、清晰度引用、Clone 和 metadata 往返，不改旧消息语义。 |
| 2. 模型图片输入 | `feat(llm): support vision inputs across protocol adapters` | `internal/llm/` 出站准备和四个协议子包、`internal/agent/tokens.go`、`compress.go` 及测试。注入解析能力和可信作用域，构造用户图片与原生工具图片结果，预留 OpenAI Chat 工具投影，接通预算与压缩后的恢复。图片功能尚未暴露在 UI 时即可用假服务验证。 |
| 3. 聊天界面接入 | `feat(chat): add image attachments to conversations` | `app/chat.go`、相关附件 binding 与内部编排、`main.go` 预览入口；前端 `ChatInput`、`ChatPanel`、`types`、`ChatTurn`、`MessageBubble`、必要的 query keys 与中英文文案。支持选择、粘贴、预览、纯图片、失败保留和历史恢复，通过命令重新生成 Wails bindings。 |
| 4. read 主动读图 | `feat(tools): let read return image attachments` | `internal/mcp_tools/base.go`、`rw_tools.go`、工具 schema / 说明；`internal/agent/agent.go`、`safety.go` 及展示映射；OpenAI Chat 工具图片投影和各协议工具结果回归；前端工具卡片按需补预览。接通结构化工具图片、归属与模型能力校验、重启后的工具历史重放。 |

阶段 2 先定义并验证工具图片的协议表达；阶段 4 接通实际工具执行与 Agent 持久化，复用同一套图片结果结构。第一阶段使用单一文件 Store，不创建空壳 Service，也不提前实现后续阶段的初始化接线。

不手工编辑 `frontend/src/lib/wailsjs/` 自动生成文件。生成后检查差异，不能把与本功能无关的生成变化混入实现。当前聊天页面仍有其他开发中的改动，实现时基于当时实际差异核对，不覆盖已有工作。

## 10. 验证与验收

### 10.1 附件、消息与预算

- PNG / JPEG、透明图、照片方向、超长图、大像素和损坏文件；标准版等比例缩小、小图不放大、原始清晰度不被二次有损处理。
- 类型识别、大小和解码像素限制；跨小说引用、非法 ID、路径穿越及软链接逃逸拒绝读取。
- 纯文本历史保持原行为；新增图片的 metadata 往返、未知字段保留、Clone 独立性、时间前缀与 append-only 语义正确。
- 删除用户原文件后仍能发图；导入或消息写入失败时无半成品消息；重复追问不重复生成处理版本。
- 实际请求预算包含历史和工具图片；Base64 不按文本 token 计算，压缩后仍超预算有明确终态。

### 10.2 协议与 Agent

- 四套协议分别验证图文、纯图片、多图的实际请求 JSON，Stream / Generate 行为一致且不修改原始消息。
- read 图片以原生内容进入模型请求，不能只出现包含 Base64 的 JSON 文本；工具调用 ID 和结果配对正确。
- OpenAI Chat 多工具组全部结果先返回，兼容图片消息后追加；没有额外落库用户消息，也没有重复图片。
- 工具失败、模型不支持视觉、缺失附件、切换协议、重试、主 / 子 Agent、摘要生成和压缩后重载均覆盖。
- 先用本地假 HTTP 服务验证出站请求；现有 SSE 回放继续验证文本、思考和工具事件不回归。没有真实视觉请求的记录样本时，不伪造厂商录制测试。

### 10.3 前端与完整链路

- 文件选择、粘贴、移除、清晰度选择、纯图片发送、失败草稿保留，以及切换小说 / 会话时的草稿隔离。
- 用户图片与工具图片在刷新、切会话和重启后可恢复；缺失图片不阻塞整段文字历史展示。
- 手工验证至少一个实际视觉端点的“发图 → 追问 → read 再读取 → 重启续聊”；其他协议如仅完成假服务验证，应明确记录实测范围。

Go 相关构建和测试从仓库根目录执行：`go build ./...`、`go test ./...`；前端从 `frontend/` 执行 `npm run build`、`npm run lint`、`npm run test`。按阶段和实际影响选择验证，最终满足对应 hook 检查；不因测试通过自动暂存、提交或推送。

## 11. 已知限制

- 自动附件回收暂缓，孤立附件可能占用空间；不以牺牲历史可恢复性换取立即清理。
- 原图加发送版本可能增加本地占用，优先换取清晰度可选和历史发送的一致性。
- 图片 token 是模型相关的估算，不能用固定字节压缩率推导费用；兼容中转站仍需验证实际端点是否接受图片。
- 首版的标准缩放不能保证所有文字长图都清晰；原始清晰度仍受请求大小、模型分辨率和上下文预算约束。
- 压缩保留范围以外的图片不会自动重新加入上下文；可通过已知附件路径再次 read，但摘要不能替代原图的全部细节。
- 第二阶段通过本地 HTTP 假服务验证四套协议的流式和非流式图片请求，包括多图、纯图片、历史追问和工具图片结果。真实视觉端点与跨平台剪贴板尚未实测；界面和 read 工具尚未接入。

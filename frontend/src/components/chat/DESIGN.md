ChatPanel
├─ ChatHeader                         标题、历史、新对话
├─ SessionHistory                     历史会话浮层
│
├─ ChatHistoryView                    消息滚动区域
│  ├─ ChatMessageList
│  │  ├─ RecentSessions               条件：最近会话列表
│  │  ├─ 空态 / 加载 / 错误重试        条件
│  │  ├─ 加载更早消息入口              条件
│  │  └─ ChatTurn × N                 每轮对话
│  │     ├─ MessageBubble             用户文字与图片
│  │     │  ├─ ChatImages              条件：有图片
│  │     │  │  └─ ChatImage × N
│  │     │  │     └─ Dialog           条件：打开大图预览
│  │     │  └─ Markdown
│  │     ├─ 工作过程折叠按钮            条件
│  │     ├─ TurnSegmentView × N        按段类型渲染
│  │     │  ├─ ThinkingBlock          思考内容
│  │     │  ├─ MessageBubble          助手回复
│  │     │  ├─ ToolCallCard           普通工具、审批
│  │     │  ├─ WebSearchCard          搜索结果
│  │     │  ├─ WebFetchCard           网页读取结果
│  │     │  ├─ CompressionBlock       上下文压缩
│  │     │  └─ SubagentCard           子 agent
│  │     │     ├─ ThinkingBlock
│  │     │     ├─ MessageBubble
│  │     │     ├─ ToolCallCard
│  │     │     └─ CompressionBlock
│  │     ├─ MessageBubble             条件：折叠后单独展示最终回复
│  │     └─ TurnOutcome               错误、重试等状态
│  └─ 滚动到底部按钮                   条件
│
├─ ChatComposer                       持有 useChatInputDraft
│  ├─ ChatInput
│  │  ├─ ChatImages                   条件：待发送图片
│  │  │  └─ ChatImage × N             移除、清晰度、预览
│  │  ├─ 导入状态 / 视觉能力提示        条件
│  │  ├─ 当前斜杠命令标记              条件
│  │  ├─ textarea                    文字输入、粘贴图片
│  │  ├─ 发送 / 停止按钮
│  │  └─ SlashMenu                    条件：斜杠命令菜单
│  ├─ 分隔线
│  └─ ChatControls
│     ├─ ChatActionsMenu
│     │  ├─ 隐藏的文件选择框
│     │  └─ DropdownMenu              + → 上传图片
│     ├─ ModelPicker                  模型、推理强度、配置入口
│     ├─ 自动审批按钮
│     └─ ContextRing                  上下文用量、压缩入口
│
├─ SettingsDialog                     模型设置弹窗
└─ DeleteSessionDialog
   └─ ConfirmDialog                  删除会话确认
<p align="center">
  <img src="assets/logo-dark.svg#gh-dark-mode-only" alt="Goink" />
  <img src="assets/logo-light.svg#gh-light-mode-only" alt="Goink" />
</p>

<h1 align="center">桌面 AI 写作系统<br><sub>Agent 实时决策 × 结构化记忆 × 写完自检状态</sub></h1>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go 1.25" />
  <img src="https://img.shields.io/badge/Wails-v2.12-DF0000?style=for-the-badge&logo=wails&logoColor=white" alt="Wails v2" />
  <img src="https://img.shields.io/badge/React-19-61DAFB?style=for-the-badge&logo=react&logoColor=white" alt="React 19" />
  <img src="https://img.shields.io/badge/SQLite-3-003B57?style=for-the-badge&logo=sqlite&logoColor=white" alt="SQLite" />
  <br />
  <img src="https://img.shields.io/badge/TypeScript-6.0-3178C6?style=for-the-badge&logo=typescript&logoColor=white" alt="TypeScript 6" />
  <img src="https://img.shields.io/badge/Tailwind-4.3-06B6D4?style=for-the-badge&logo=tailwindcss&logoColor=white" alt="Tailwind 4" />
  <img src="https://img.shields.io/badge/ONNX_Runtime-1.26-005BED?style=for-the-badge&logo=onnx&logoColor=white" alt="ONNX Runtime" />
  <img src="https://img.shields.io/badge/license-AGPL_v3-blue?style=for-the-badge&logo=opensourceinitiative&logoColor=white" alt="AGPL v3" />
</p>

<p align="center"><strong><a href="README_EN.md">English Version</a> | 本文档为中文版</strong></p>

---

<p align="center"><strong>用过通用 AI 写长篇小说的人都知道——写到第五章它就忘了主角叫什么。到第三十章还要手动翻前文找那句伏笔。写完一章还得自己提醒它"更新角色状态""检查弧线进度"。Goink 不会。它是一个有结构化记忆的桌面 AI 写作系统——角色档案、伏笔状态、弧线进度、地点关系、读者认知，系统记着，Agent 自己查、自己改、自己维护。</strong></p>

## 跟通用 AI 聊天有什么不同

| | 通用 AI 聊天 | Goink |
|---|---|---|
| 创作信息 | 每次对话重新交代 | 角色/关系/伏笔/弧线/地点/读者认知 全链路结构化追踪 |
| 改正文 | 直接输出文本，改了什么不知道 | Diff 预览 + 逐行对比 + 点确认才写入 |
| 翻前文 | 手动搜索、逐章翻 | 本地语义搜索引擎，一句"那个吊坠"找到所有段落 |
| 写完维护 | 不管，除非你再提醒 | 写完自动触发角色更新、伏笔回收、弧线推进、读者认知刷新 |
| 写作风格 | 靠 prompt 硬写 | 8 个内置方法论 + 自定义 Skill 热重载，三层覆盖 |
| 版本历史 | 无 | 内置 Git，每次对话自动 commit，随时回退 |
| 环境依赖 | 往往要 Python/GPU | 一个安装包，打开即用 |

## AI 自己查、自己改、自己维护——不是流水线，是 Agent

31 个结构化工具，LLM 自主决策调用哪个、传什么参数、下一步干什么。不是"写完一章传给下一棒"的 pipeline——Agent 在当前对话中调工具查角色、查伏笔、读写正文、更新状态，直到任务完成。

写完一章正文后，系统自动注入维护提醒，告诉 Agent 具体检查什么：角色有没有变化、该回收的伏笔回收了没有、弧线节点需要推进吗、读者认知需要更新吗。Agent 不会"忘了维护"——它被迫逐项自查。

如果还不放心，可以启动审稿子 Agent——一个独立 Agent 从头审读章节内容与系统状态的一致性，发现问题直接写进对话，主 Agent 当场修正。

## 几十万字里找一句话：本地语义搜索

写到第五十章，要找"主角第一次见到那个吊坠是在哪一章来着？"——不用逐章翻。告诉 AI 一句话，它能在整本书里找到相关段落。

不是关键词匹配，是按意思搜索。你问"关于吊坠的线索"，它能找到那些没写"吊坠"两个字但确实在暗示吊坠存在的段落。Agent 写新章节时也可主动搜索前文，确保持续一致。

整套引擎在本机运行——BGE 中文语义模型 ONNX 本地推理，sqlite-vec 向量索引，MMR 去冗余重排序。写完章节后台自动增量索引，无需网络，无需额外配置。

## 不只是记忆——是结构化创作状态

### 角色：关系有历史

角色档案包含性格、能力、背景。角色关系是有向图——"张三对李四是师徒但暗中提防"，"李四对张三是敬重但有所隐瞒"，两条独立记录。关系变化时旧记录保留，可回顾演变过程。

### 伏笔：不会石沉大海

每条伏笔记录目标回收章节和重要程度。快到回收点系统提醒，超时未回收标记异常。章节计划分三档——下一章、近期、远期——管理创作节奏。

### 弧线：跨章节叙事线索

弧线由节点链组成，每个节点关联目标章节。写完一章自动推进节点。一个故事通常 3–5 条并行弧线同时追踪。

### 世界观：地点是图，不是列表

追踪层级包含（王国 → 王宫 → 大殿）和空间连通（A 和 B 由山路连通）。AI 可查详情、子地点、连通关系或完整地图。

### 读者认知：控制信息释放

追踪读者已知什么、在等什么答案、误认了什么。精确控制悬念和反转时机。

### 创作偏好：说一次就够

全局偏好和单书偏好两层管理。写到第三十七章，"对话保持冷峻风格"依然生效。

## 前端可视化状态
<p align="center">
  <img src="assets/arc-demo.png" alt="故事弧线" />
</p>
<p align="center">
  <img src="assets/location-demo.png" alt="地点图谱" />
</p>
<p align="center">
  <img src="assets/preferences-demo.png" alt="创作偏好" />
</p>

## Skill 系统：3 层覆盖 × 3 种模式

> [!TIP]
> **欢迎贡献你的 Skill！** 把你的写作方法论变成 `.md` 文件，[提交 PR](https://github.com/sigpanic/goink-skills) 分享给所有用户。

Skill 是 Goink 的创作方法论模块。每个 Skill 由一个 `.md` 文件定义，包含 YAML frontmatter 元数据和 markdown 正文。**三层覆盖 + 三种模式 = 9 种策略维度**，精确控制"什么内容、在什么范围、以什么方式生效"。

### 三层覆盖

同名 Skill 按 **小说 > 用户 > 内置** 优先级覆盖。修改即时热重载，无需重启。

| 层级 | 存储路径 | 可见范围 | 可编辑 |
|---|---|---|---|
| 内置 Builtin | 打包只读 | 所有小说 | 否 |
| 用户 User | `~/.goink/skills/` | 所有小说 | 是 |
| 小说 Novel | `{novel}/skills/` | 当前小说 | 是 |

### 三种触发模式

| 模式 | AI 自主调用 | 用户 `/` 触发 | 会话开头注入 | 出现在目录 |
|---|---|---|---|---|
| 智能 `auto` | 是 | 是 | — | 是 |
| 指令 `manual` | — | 是 | — | — |
| 常驻 `always` | 是 | 是 | 是（注入全文） | — |

### 3×3 能力矩阵

|  | 智能 auto | 指令 manual | 常驻 always |
|---|---|---|---|
| **内置** | 场景节拍、对白潜台词、节奏控制、悬念钩子、角色设计、修改打磨、去AI味、共创构思 | review / memory / collect / next | — |
| **用户** | 跨小说可复用的创作工作流 | 个人快捷命令 | 全局生效的风格规则 |
| **小说** | 单书专属工作流 | 单书快捷命令 | 单书常驻规则 |

新建一个 `.md` 文件就是新 Skill：

```markdown
---
name: 我的写作流程
description: 个人定制创作流程
category: 自定义
mode: auto
---
# 正文 markdown 内容
```

零代码扩展。修改即时生效。删除同理。

<p align="center">
  <img src="assets/skill-demo.png" width="80%" alt="Skill 技能系统" />
</p>

## 风格蒸馏：一段文字 → 一个仿写 Skill

想写出某个作家的笔法？贴一段样文，AI 从六个维度拆解——**句式结构、用词习惯、修辞手法、节奏控制、叙事距离、氛围语调**——自动生成一个完整的仿写 Skill。不是关键词替换，是提炼风格模式。

生成的 Skill 立刻出现在列表中，`/风格名` 一键加载，后续所有对话都按此风格输出。也可以打开编辑继续微调。

<p align="center">
  <img src="assets/extract-demo.png" width="80%" alt="风格蒸馏" />
</p>

## 三重保障，维护不会遗漏

**第一层—系统提示词** • Agent 核心指令写死维护流程。"创作完成后立即进行状态维护。不是可选步骤。"

**第二层—动态注入** • AI 写完长文后系统自动注入检查项——角色变化、伏笔状态、弧线节点、读者认知。

**第三层—审稿 Agent** • 独立子 Agent 对比章节与系统状态，发现问题立即反馈。

## 你的每一次确认

AI 不会直接改正文。每次编辑系统先生成 Diff，等你批准再写入。可以当场批准、拒绝，或者给反馈让 AI 修正。也可以切换到自动模式，连续多轮自由写作。

所有修改都有 Git 历史，任何时候都可以回退到任意状态。
<p align="center">
  <img src="assets/write-demo.png" alt="写作与 Diff 审批" />
</p>
<p align="center">
  <img src="assets/outline-demo.png" alt="大纲与章节计划" />
</p>
## AI 碰不到不该碰的文件

双层沙箱安全隔离——正则白名单只允许 `chapters/`、`outlines/`、`goink.md` 等合法路径，SafePath 杜绝路径穿越。文件编辑写入前重读对比，防止覆盖你的手动修改。

## 安装

从 [Releases](https://github.com/sigpanic/goink/releases) 下载对应平台安装包：

- **Windows** — 运行安装程序
- **macOS** — 打开 DMG，拖入 Applications
- **Linux** — 运行 AppImage

需要 LLM API Key（内置 DeepSeek、GLM、MiMo 模板，兼容 OpenAI 格式）。安装包 < 60MB，不需要 Python、Node.js、数据库或 GPU。Windows SmartScreen 可能弹出提示（未签名），点击"更多信息"→"仍要运行"即可。

> 网络不好？可从网盘下载：[https://wwayx.lanzouu.com/b0kp2iyoj](https://wwayx.lanzouu.com/b0kp2iyoj) 密码：1111

### 从源码构建

```bash
sudo apt install libsqlite3-dev libgtk-3-dev libwebkit2gtk-4.1-dev gcc
git clone https://github.com/sigpanic/goink
cd goink
make deps
make build   # 生产构建
make dev     # 开发模式（热重载）
```

## 技术栈

| 层 | 选型 |
|---|---|
| Agent 引擎 | 自研 ReAct 循环（Go，SSE 流式 + 31 个 Function Calling 工具 + 子 Agent 嵌套） |
| 桌面框架 | Wails v2（Go + WebView） |
| 编辑器 | Monaco Editor |
| 数据库 | SQLite + GORM（ACID 事务 + 操作日志回滚） |
| 向量搜索 | sqlite-vec + ONNX Runtime（bge-small-zh-v1.5 int8 量化） |
| 版本控制 | 内置 Git（自动 commit / Diff / Revert） |
| 安全 | 正则白名单 + SafePath 双层沙箱 + 审批流 |
| 前端 | React 19 + TypeScript + Tailwind CSS 4 + shadcn/ui |

## License

Goink is licensed under the GNU Affero General Public License v3.0 (AGPL-3.0). See [LICENSE](LICENSE) for the full text and [NOTICE](NOTICE) for additional terms under AGPLv3 Section 7.

## Star History

<a href="https://www.star-history.com/#repos=sigpanic%2Fgoink&type=Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=sigpanic/goink&type=date&theme=dark&legend=top-left" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=sigpanic/goink&type=date&legend=top-left" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=sigpanic/goink&type=date&legend=top-left" />
 </picture>
</a>

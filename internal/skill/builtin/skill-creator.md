---
name: skill-creator
description: 当要创建新的 skill 或修改已有的 skill 时，必须立刻调用本 skill。提供技能文件格式、写入层级和编写修改规范。
category: 系统
mode: auto
author: builtin
version: 1
---
# 技能编写

## 写入位置

- 新技能默认写入当前小说的 `skills/<name>.md`。
- 用户要求跨小说复用时，写入用户级路径 `~/.goink/skills/<name>.md`。
- 用户已指定层级或路径时，遵循用户要求。内置技能 `/builtin/skills/<name>.md` 只读，不可编辑。
- 文件名应与 frontmatter 的 `name` 一致。

## 文件格式

使用 YAML frontmatter + Markdown 正文，填写 `name`、`description`、`category`、`mode`：

```markdown
---
name: my-workflow
description: 整理章节修订步骤，在用户要求按固定流程修改章节时使用。
category: 修订
mode: auto
---

# 章节修订工作流

先检查结构和情节因果，再调整场景节奏，最后打磨句子。
```

`mode` 根据用户希望的生效方式选择：

- `auto`：默认选择，适合在请求匹配时按需加载的工作流。
- `manual`：适合仅由用户通过 `/技能名` 触发的快捷指令。
- `always`：适合需要在每次对话中常驻生效的规则，不用于只在特定任务中需要的详细流程。

`author`、`version` 为可选元数据，修改已有技能时保留现有值，除非用户要求调整。

## 编写与修改

- `description` 简短说明技能做什么、何时使用；将具体步骤放在正文中，避免用宽泛关键词吸引无关请求。
- 正文写清可复用的工作方法、必要工具和完成条件，不把某一章的一次性信息变成长期规则。
- 修改已有技能前，先用 `read` 读取原文，保留仍然适用的内容和元数据。创建时也应先检查目标路径，避免覆盖同名技能。
- 必须通过 `edit` 工具写入文件，不能只在回复中展示技能内容。新建使用 `full_replace`，修改可使用 `full_replace`、`search_replace` 或 `line_range_replace`。

新建小说级技能的调用形式：

`edit(path="skills/<name>.md", change_type="full_replace", new_content="<包含 frontmatter 的完整 Markdown>")`

用户级技能使用相同调用形式，将 `path` 改为 `~/.goink/skills/<name>.md`。

写入前检查四个字段齐全、`name` 与文件名一致、`mode` 为上述三个值之一；工具返回格式错误时修正后重试。遵循 `edit` 的审批结果，只有工具确认写入成功后才向用户报告完成。

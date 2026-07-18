// Package remote 提供远程 skill 市场功能。
//
// 通过 GitHub Contents API 拉取 goink-skills 仓库的 index.json 和 skill 文件，
// 支持本地缓存、安装到用户层或小说层 skill 目录，并触发热重载。
// 网络错误按语义类别（githubapi.Kind）透传给调用方，便于前端给出针对性提示。
package remote

// IndexFile 对应远程仓库 goink-skills/index.json 的结构。
// 该文件由仓库内的 scripts/generate-index.py 生成。
type IndexFile struct {
	Updated string            `json:"updated"` // ISO 8601 UTC 时间戳
	Skills  []RemoteSkillMeta `json:"skills"`
}

// RemoteSkillMeta 是远程 skill 的元数据，字段对齐 generate-index.py 输出。
type RemoteSkillMeta struct {
	Name        string `json:"name"`        // skill 名称，同时作为文件名（不含 .md 后缀）
	Description string `json:"description"` // 简要描述
	Category    string `json:"category"`    // 分类
	Mode        string `json:"mode"`        // 触发模式：auto / manual / always
	Author      string `json:"author"`      // 作者
	Version     int    `json:"version"`     // 版本号（整数，缺失时默认 1）
	File        string `json:"file"`        // 文件名，如 "my-skill.md"
}

package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sigpanic/goink/internal/apperr"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/skill"
	"github.com/sigpanic/goink/internal/skill/remote"
	"github.com/sigpanic/goink/internal/storage"
)

// ListSkillsInput 是 ListSkills 的入参。
type ListSkillsInput struct {
	NovelID int64 `json:"novel_id"`
}

// ListSkills 返回所有可用 skill 的元数据（同名覆盖：novel > user > builtin）。
func (a *App) ListSkills(input ListSkillsInput) []skill.SkillMeta {
	if a.skill == nil {
		return nil
	}
	return a.skill.ListMeta(input.NovelID)
}

// DeleteSkillInput 是 DeleteSkill 的入参。
type DeleteSkillInput struct {
	NovelID int64  `json:"novel_id"`
	Name    string `json:"name"`
	Source  string `json:"source"` // "novel" | "user"
}

// DeleteSkill 删除用户级或小说级技能文件。内置技能不可删除。
func (a *App) DeleteSkill(input DeleteSkillInput) error {
	if a.skill == nil {
		return fmt.Errorf("skill store 未初始化")
	}
	if input.Name == "" {
		return fmt.Errorf("技能名称不能为空")
	}
	name := strings.TrimSuffix(filepath.Base(input.Name), ".md")
	if name == "" || name != input.Name {
		return fmt.Errorf("技能名称非法")
	}

	source := input.Source
	if source != "novel" && source != "user" {
		return fmt.Errorf("只能删除用户级或小说级技能")
	}

	var filePath string
	switch source {
	case "novel":
		if input.NovelID <= 0 {
			return fmt.Errorf("小说 ID 无效")
		}
		var err error
		filePath, err = git.SafePath(config.NovelSkillsDir(input.NovelID), name+".md")
		if err != nil {
			return fmt.Errorf("技能名称非法: %w", err)
		}
	case "user":
		var err error
		filePath, err = git.SafePath(config.UserSkillsDir(), name+".md")
		if err != nil {
			return fmt.Errorf("技能名称非法: %w", err)
		}
	}

	if err := os.Remove(filePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("技能文件不存在: %s", name)
		}
		return fmt.Errorf("删除技能文件失败: %w", err)
	}

	// 重新加载对应层级
	switch source {
	case "novel":
		if err := a.skill.ReloadNovel(input.NovelID, config.NovelSkillsDir(input.NovelID)); err != nil {
			a.logger.Warn("删除技能后重新加载小说级技能失败", "name", name, "err", err)
		}
	case "user":
		if err := a.skill.ReloadUser(config.UserSkillsDir()); err != nil {
			a.logger.Warn("删除技能后重新加载用户级技能失败", "name", name, "err", err)
		}
	}

	return nil
}

// ListRemoteSkillsInput 是 ListRemoteSkills 的入参。
// Page/Size 走 storage.PageParams 归一化（Page<1 → 1，Size 默认 20，上限 100）。
// Query 为 name + description 的模糊匹配（大小写不敏感），空串表示不过滤。
type ListRemoteSkillsInput struct {
	Page  int    `json:"page"`
	Size  int    `json:"size"`
	Query string `json:"query"`
}

// ListRemoteSkills 列出远程 skill 市场的所有 skill，支持分页和搜索。
// forceRefresh=false 优先使用内存缓存（1h TTL），前端"刷新"按钮可改为 true 强制刷新。
// 错误以 *apperr.Result[*storage.PageResult[remote.RemoteSkillMeta]] 透传，前端按 err_code 分类反馈。
func (a *App) ListRemoteSkills(input ListRemoteSkillsInput) *apperr.Result[*storage.PageResult[remote.RemoteSkillMeta]] {
	all, err := a.remote.ListRemoteSkills(a.ctx, false)
	if err != nil {
		return apperr.Err[*storage.PageResult[remote.RemoteSkillMeta]](err)
	}

	// 1. 搜索过滤（name + description 模糊匹配，大小写不敏感）
	filtered := filterRemoteSkillsByQuery(all, input.Query)

	// 2. 分页（复用 storage.PageParams.Normalize + storage.NewPageResult）
	p := (&storage.PageParams{Page: input.Page, Size: input.Size}).Normalize()
	start := (p.Page - 1) * p.Size
	end := start + p.Size
	if start > len(filtered) {
		start = len(filtered)
	}
	if end > len(filtered) {
		end = len(filtered)
	}
	page := storage.NewPageResult(filtered[start:end], int64(len(filtered)), p.Page, p.Size)

	return apperr.Ok(page)
}

// filterRemoteSkillsByQuery 按 query 模糊匹配 name + description（大小写不敏感）。
// query 为空串时返回全部。
func filterRemoteSkillsByQuery(skills []remote.RemoteSkillMeta, query string) []remote.RemoteSkillMeta {
	if query == "" {
		return skills
	}
	q := strings.ToLower(query)
	out := make([]remote.RemoteSkillMeta, 0, len(skills))
	for _, s := range skills {
		if strings.Contains(strings.ToLower(s.Name), q) ||
			strings.Contains(strings.ToLower(s.Description), q) {
			out = append(out, s)
		}
	}
	return out
}

// GetRemoteSkillContent 拉取指定远程 skill 的 markdown 原文内容。
// 用于详情面板展示全文。
func (a *App) GetRemoteSkillContent(name string) *apperr.Result[string] {
	content, err := a.remote.GetRemoteSkillContent(a.ctx, name)
	if err != nil {
		return apperr.Err[string](err)
	}
	return apperr.Ok(content)
}

// InstallRemoteSkillInput 是 InstallRemoteSkill 的入参。
// Target 取值 "user" 或 "novel"；Target=novel 时 NovelID 必填。
type InstallRemoteSkillInput struct {
	Name    string `json:"name"`
	Target  string `json:"target"`   // "user" or "novel"
	NovelID int64  `json:"novel_id"` // target=novel 时必填
}

// InstallRemoteSkill 将指定远程 skill 安装到目标层（user 或 novel）。
// 安装成功后触发 skill.Store 热重载（失败只 Warn 不返回 error）。
// 后端不做存在性判断，前端弹确认框处理覆盖语义。
func (a *App) InstallRemoteSkill(input InstallRemoteSkillInput) *apperr.Result[apperr.Empty] {
	if err := a.remote.InstallRemoteSkill(a.ctx, input.Name, input.Target, input.NovelID); err != nil {
		return apperr.Err[apperr.Empty](err)
	}
	return apperr.Ok(apperr.Empty{})
}

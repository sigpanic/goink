package app

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/sigpanic/goink/internal/apperr"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/skill"
	"github.com/sigpanic/goink/internal/utils"
)

// MoveSkillInput 是 MoveSkill 的入参。
type MoveSkillInput struct {
	NovelID int64  `json:"novel_id"`
	Name    string `json:"name"`
	From    string `json:"from"` // "novel" | "user"
	To      string `json:"to"`   // "novel" | "user"
}

// MoveSkill 将技能在小说级和用户级之间移动。
// 目标层已存在同名技能时返回 skill.exists，不覆盖。
func (a *App) MoveSkill(input MoveSkillInput) *apperr.Result[skill.SkillMeta] {
	if a.skill == nil {
		return apperr.Err[skill.SkillMeta](errors.New("skill store 未初始化"))
	}
	if err := skill.ValidateName(input.Name); err != nil {
		return moveSkillError(skill.CodeInvalidName, "技能名称非法", err)
	}
	if input.From != "novel" && input.From != "user" {
		return moveSkillError(skill.CodeInvalidScope, "源层级非法", nil)
	}
	if input.To != "novel" && input.To != "user" {
		return moveSkillError(skill.CodeInvalidScope, "目标层级非法", nil)
	}
	if input.From == input.To {
		return moveSkillError(skill.CodeInvalidScope, "源层级和目标层级相同", nil)
	}
	if (input.From == "novel" || input.To == "novel") && input.NovelID <= 0 {
		return moveSkillError(skill.CodeInvalidScope, "小说 ID 无效", nil)
	}

	srcDir := config.UserSkillsDir()
	dstDir := config.UserSkillsDir()
	if input.From == "novel" {
		srcDir = config.NovelSkillsDir(input.NovelID)
	}
	if input.To == "novel" {
		dstDir = config.NovelSkillsDir(input.NovelID)
	}

	srcPath, err := git.SafePath(srcDir, input.Name+".md")
	if err != nil {
		return moveSkillError(skill.CodeInvalidName, "技能名称非法", err)
	}
	dstPath, err := git.SafePath(dstDir, input.Name+".md")
	if err != nil {
		return moveSkillError(skill.CodeInvalidName, "技能名称非法", err)
	}

	sk, err := skill.Move(srcPath, dstPath, input.Name)
	if err != nil {
		switch {
		case errors.Is(err, skill.ErrInvalidName):
			return moveSkillError(skill.CodeInvalidName, "技能名称非法", err)
		case errors.Is(err, skill.ErrInvalidContent):
			return moveSkillError(skill.CodeInvalidContent, "技能内容非法", err)
		case errors.Is(err, utils.ErrPartialMove):
			return moveSkillError(skill.CodeMovePartial, "目标文件已写入，但源文件删除失败", err)
		case errors.Is(err, fs.ErrExist):
			return moveSkillError(skill.CodeExists, "目标层级已存在同名技能", err)
		case errors.Is(err, fs.ErrNotExist):
			return moveSkillError(skill.CodeNotFound, "源技能不存在", err)
		default:
			return apperr.Err[skill.SkillMeta](fmt.Errorf("移动技能 %q 失败: %w", input.Name, err))
		}
	}

	reload := func(source string) {
		if source == "novel" {
			if err := a.skill.ReloadNovel(input.NovelID, config.NovelSkillsDir(input.NovelID)); err != nil {
				a.logger.Warn("移动技能后重新加载小说级技能失败", "name", input.Name, "err", err)
			}
			return
		}
		if err := a.skill.ReloadUser(config.UserSkillsDir()); err != nil {
			a.logger.Warn("移动技能后重新加载用户级技能失败", "name", input.Name, "err", err)
		}
	}
	reload(input.From)
	reload(input.To)

	return apperr.Ok(sk.Meta(input.To))
}

func moveSkillError(code apperr.Code, msg string, cause error) *apperr.Result[skill.SkillMeta] {
	return apperr.Err[skill.SkillMeta](&apperr.BusinessError{CodeVal: code, Msg: msg, Cause: cause})
}

package agentcfg

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/skill"
)

// SystemMessages 是注入对话的 system 消息集合。
// chat 和 compress 共用此结构，避免重复构建逻辑（呼应踩坑 16）。
type SystemMessages struct {
	Identity string // AgentIdentity（system1 提示词）
	Always   string // 常驻技能正文
	Catalog  string // auto 模式技能目录
	Profile  string // NovelProfile（设定+偏好，稳定）
	State    string // NovelState（基础信息+goink.md，动态）
}

// BuildSystemMessages 统一构建 main agent 的 system 消息。
// chat 和 compress 共用此函数，避免重复实现。
//
// 全部在事务外构建（compress 模式）：传入的 db 应为带 ctx 的非事务句柄，
// 避免在事务内调用导致 SQLite 单连接池死锁（呼应踩坑 13/15）。
// skillStore 为 nil 时跳过 Always/Catalog。
//
// 容错：Profile/State 构建失败时对应字段为空（写入时跳过），不阻断对话。
// 返回的 error 是聚合错误，调用方可用于 log，但 msg 仍可使用。
//
// 注意：本函数固定使用 MainAgent 的 Identity，不适用于 subagent。
// subagent 路径请直接调 AgentIdentity(at) + NovelProfile + NovelState。
func BuildSystemMessages(ctx context.Context, db *gorm.DB, novelID int64, skillStore *skill.Store) (*SystemMessages, error) {
	msg := &SystemMessages{
		Identity: AgentIdentity(MainAgent),
	}

	if skillStore != nil {
		all := skillStore.ListMeta(novelID)
		msg.Catalog = BuildSkillCatalog(skillStore.ListMetaForCatalog(all))
		msg.Always = BuildAlwaysSkillsContent(all, skillStore, novelID)
	}

	var errs []string

	if profile, err := NovelProfile(ctx, db, novelID); err != nil {
		errs = append(errs, fmt.Sprintf("novel profile: %v", err))
	} else {
		msg.Profile = profile
	}

	if state, err := NovelState(ctx, db, novelID); err != nil {
		errs = append(errs, fmt.Sprintf("novel state: %v", err))
	} else {
		msg.State = state
	}

	var err error
	if len(errs) > 0 {
		err = fmt.Errorf("build system messages: %s", strings.Join(errs, "; "))
	}
	return msg, err
}

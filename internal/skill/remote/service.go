package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sigpanic/goink/internal/apperr"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/githubapi"
	"github.com/sigpanic/goink/internal/skill"
)

// 远程 skill 仓库标识。
const (
	repoOwner = "sigpanic"
	repoName  = "goink-skills"
	branch    = "main"
	indexPath = "index.json"
	skillsDir = "skills"  // 仓库内 skills 目录
	cacheTTL  = time.Hour // 内存缓存有效期
)

// rawContentFetcher 抽象 GitHub Contents API 的 raw 内容拉取能力。
// *githubapi.Client 自动满足该接口，测试时可注入 mock 实现以隔离 HTTP 层。
type rawContentFetcher interface {
	GetRawContent(ctx context.Context, owner, repo, branch, path string) ([]byte, *githubapi.RateLimit, error)
}

// skillReloader 抽象 skill.Store 的热重载能力。
// *skill.Store 自动满足该接口，测试时可注入 mock 实现以隔离文件系统。
type skillReloader interface {
	ReloadUser(userSkillsDir string) error
	ReloadNovel(novelID int64, novelSkillsDir string) error
}

// Service 提供远程 skill 市场的业务逻辑：列表、查看、安装。
//
// 持有 GitHub API 客户端、skill 仓库热重载句柄、目录解析器和内存缓存。
// 网络错误透传 *githubapi.Error，调用方可按 Kind 分类处理。
type Service struct {
	client      rawContentFetcher
	skillStore  skillReloader
	logger      *slog.Logger
	dirResolver func(target string, novelID int64) (string, error)

	// 内存缓存：cacheSkills 为 nil 表示无缓存；cacheAt 为写入时间。
	// 命中条件：cacheSkills != nil && time.Since(cacheAt) < cacheTTL。
	// cacheMu 保护 cacheSkills 和 cacheAt 的并发读写，调 API 时不持锁。
	cacheMu     sync.RWMutex
	cacheSkills []RemoteSkillMeta
	cacheAt     time.Time
}

// NewService 创建生产环境使用的 Service。
// skillStore 传入 *skill.Store，内部用 githubapi.NewClient() 拉取远程内容。
// 缓存为进程内内存缓存，TTL 由 cacheTTL 控制，进程重启后失效。
func NewService(skillStore *skill.Store, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		client:      githubapi.NewClient(),
		skillStore:  skillStore,
		logger:      logger,
		dirResolver: defaultDirResolver,
	}
}

// newServiceWithClient 创建测试用 Service，允许注入 mock client、mock reloader、
// 自定义目录解析器，便于在不污染真实文件系统的前提下覆盖业务逻辑。
func newServiceWithClient(client rawContentFetcher, skillStore skillReloader, logger *slog.Logger, dirResolver func(target string, novelID int64) (string, error)) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		client:      client,
		skillStore:  skillStore,
		logger:      logger,
		dirResolver: dirResolver,
	}
}

// defaultDirResolver 是生产环境的目录解析器，根据 target 决定安装目录。
// target 取值：
//   - "user"  → config.UserSkillsDir()
//   - "novel" → config.NovelSkillsDir(novelID)，要求 novelID != 0
//
// 其他值返回错误。
func defaultDirResolver(target string, novelID int64) (string, error) {
	switch target {
	case "user":
		return config.UserSkillsDir(), nil
	case "novel":
		if novelID == 0 {
			return "", apperr.NewInvalid("remote: install to novel layer requires non-zero novelID")
		}
		return config.NovelSkillsDir(novelID), nil
	default:
		return "", apperr.NewInvalid(fmt.Sprintf("remote: invalid target %q (want user or novel)", target))
	}
}

// ListRemoteSkills 列出远程仓库的全部 skill 元数据。
//
// forceRefresh=false 时优先使用内存缓存；缓存不存在/已过期或 forceRefresh=true 时
// 调 GitHub API 拉取 index.json 并更新内存缓存。
// 错误用 fmt.Errorf("remote: ...: %w", err) 包装，保留 *githubapi.Error 的 Unwrap 链。
//
// 并发安全：cacheMu 保护 cacheSkills/cacheAt 读写，调 API 时不持锁。
func (s *Service) ListRemoteSkills(ctx context.Context, forceRefresh bool) ([]RemoteSkillMeta, error) {
	// 读锁检查缓存命中：非强制刷新 + 有缓存 + 未过期。
	s.cacheMu.RLock()
	if !forceRefresh && s.cacheSkills != nil && time.Since(s.cacheAt) < cacheTTL {
		skills := s.cacheSkills
		s.cacheMu.RUnlock()
		return skills, nil
	}
	s.cacheMu.RUnlock()

	// 调 API 时不持锁，避免网络 I/O 阻塞其他读。
	body, _, err := s.client.GetRawContent(ctx, repoOwner, repoName, branch, indexPath)
	if err != nil {
		return nil, fmt.Errorf("remote: fetch index.json: %w", err)
	}

	var idx IndexFile
	if err := json.Unmarshal(body, &idx); err != nil {
		return nil, fmt.Errorf("remote: parse index.json: %w", err)
	}

	// 写锁更新缓存。
	s.cacheMu.Lock()
	s.cacheSkills = idx.Skills
	s.cacheAt = time.Now()
	s.cacheMu.Unlock()

	return idx.Skills, nil
}

// GetRemoteSkillContent 拉取指定 skill 的 markdown 原文内容。
// 路径形如 "skills/{name}.md"。
func (s *Service) GetRemoteSkillContent(ctx context.Context, name string) (string, error) {
	path := fmt.Sprintf("%s/%s.md", skillsDir, name)
	body, _, err := s.client.GetRawContent(ctx, repoOwner, repoName, branch, path)
	if err != nil {
		return "", fmt.Errorf("remote: fetch skill %s: %w", name, err)
	}
	return string(body), nil
}

// InstallRemoteSkill 将指定远程 skill 安装到目标层（user 或 novel）。
//
// 流程：
//  1. 拉取 skill 内容
//  2. 解析目标目录（user → ~/.goink/skills/，novel → {novel_dir}/skills/）
//  3. MkdirAll + WriteFile 写入 {name}.md
//  4. 触发热重载（失败只 Warn，不返回 error，因为文件已成功写入）
//
// 不做存在性判断，前端弹确认框处理覆盖语义。
func (s *Service) InstallRemoteSkill(ctx context.Context, name, target string, novelID int64) error {
	// 路径校验：与 app.DeleteSkill 一致，name 必须是纯文件名（不含路径分隔符/后缀），
	// 防止远程 index.json 被污染时 name 含 ../ 逃出 skills 目录写任意文件
	safeName := strings.TrimSuffix(filepath.Base(name), ".md")
	if safeName == "" || safeName != name {
		return apperr.NewInvalid(fmt.Sprintf("remote: invalid skill name %q", name))
	}

	content, err := s.GetRemoteSkillContent(ctx, name)
	if err != nil {
		return err
	}

	dir, err := s.dirResolver(target, novelID)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("remote: create skill dir %s: %w", dir, err)
	}

	dst, err := git.SafePath(dir, name+".md")
	if err != nil {
		return &apperr.BusinessError{CodeVal: apperr.CodeInvalid, Msg: "remote: invalid skill path", Cause: err}
	}
	if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
		return fmt.Errorf("remote: write skill file %s: %w", dst, err)
	}

	// 文件已成功写入，热重载失败只记录日志，不影响安装结果。
	switch target {
	case "user":
		if err := s.skillStore.ReloadUser(dir); err != nil {
			s.logger.Warn("remote: reload user skills failed", "dir", dir, "err", err)
		}
	case "novel":
		if err := s.skillStore.ReloadNovel(novelID, dir); err != nil {
			s.logger.Warn("remote: reload novel skills failed", "novelID", novelID, "dir", dir, "err", err)
		}
	}

	return nil
}

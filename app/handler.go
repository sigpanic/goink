package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/agent"
	"github.com/sigpanic/goink/internal/approval"
	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/character"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/location"
	"github.com/sigpanic/goink/internal/mcp_tools"
	"github.com/sigpanic/goink/internal/migrate"
	"github.com/sigpanic/goink/internal/novel"
	"github.com/sigpanic/goink/internal/preference"
	"github.com/sigpanic/goink/internal/rag"
	"github.com/sigpanic/goink/internal/reader"
	"github.com/sigpanic/goink/internal/rollback"
	"github.com/sigpanic/goink/internal/search"
	"github.com/sigpanic/goink/internal/session"
	"github.com/sigpanic/goink/internal/setting"
	"github.com/sigpanic/goink/internal/skill"
	"github.com/sigpanic/goink/internal/skill/remote"
	"github.com/sigpanic/goink/internal/storage"
	"github.com/sigpanic/goink/internal/storyarc"
	"github.com/sigpanic/goink/internal/style"
	"github.com/sigpanic/goink/internal/timeline"
	"github.com/sigpanic/goink/internal/writing"
)

// App 是 Wails 绑定的根对象。前端通过 window.go.main.App 调用其导出方法。
// 各领域方法按文件拆分（novel.go / chapter.go 等），均接收 *App。
type App struct {
	ctx    context.Context
	cancel context.CancelFunc
	logger *slog.Logger

	cfg      *config.AppConfig
	settings *config.AppSettings
	db       *gorm.DB

	llmClient     *llm.Client
	agent         *agent.Agent
	cancelMgr     *agent.CancelManager
	registry      *mcp_tools.Registry
	approvals     *approval.Service
	vectorStore   *rag.VectorStore
	searchService atomic.Pointer[search.Service]

	novel      *novel.Store
	preference *preference.Store
	setting    *setting.Store
	chapter    *chapter.Store
	character  *character.Store
	session    *session.Store
	skill      *skill.Store
	// remote 持有远程 skill 市场服务，用于 ListRemoteSkills / GetRemoteSkillContent / InstallRemoteSkill。
	remote     *remote.Service
	style      *style.Store
	timeline   *timeline.Store
	storyarc   *storyarc.Store
	location   *location.Store
	reader     *reader.Store
	turnCommit *rollback.Store
	writing    *writing.Store
}

// New 创建 App 实例。初始化在 OnStartup 中完成。
func New(logger *slog.Logger) *App {
	return &App{logger: logger}
}

// ── 生命周期 ──────────────────────────────────────────────

// OnStartup 在 Wails 窗口创建后调用，完成基础设施初始化。
func (a *App) OnStartup(ctx context.Context) {
	a.ctx, a.cancel = context.WithCancel(ctx)

	cfg, err := config.Load()
	if err != nil {
		if errors.Is(err, config.ErrNotInitialized) {
			// 首次启动，不自动初始化，等前端展示 InitView 再由用户手动触发
			return
		}
		a.logger.Error("加载配置失败", "err", err)
		return
	}
	a.initWithConfig(cfg)
}

// OnShutdown 在 Wails 窗口关闭前调用，释放资源。
func (a *App) OnShutdown(shutdownCtx context.Context) {
	a.logger.Info("应用关闭，释放资源")

	// 1. 取消根上下文，通知所有运行中的 agent 停止
	if a.cancel != nil {
		a.cancel()
	}

	// 2. 停止 RAG 后台消费者
	if q := rag.GetRefreshQueue(); q != nil {
		q.Stop()
	}
	// 3. 释放 ONNX embedder（非阻塞，避免未初始化时死锁）
	if emb := rag.TryGetEmbedder(); emb != nil {
		_ = emb.Close()
	}

	// 4. 关闭数据库（放在最后，确保上述清理中的 DB 操作已完成）
	if a.db != nil {
		if err := storage.Close(a.db); err != nil {
			a.logger.Error("关闭数据库失败", "err", err)
		}
	}
}

// IsInitialized 返回指针文件是否已加载成功。前端据此决定显示初始化界面还是主界面。
func (a *App) IsInitialized() bool {
	return a.cfg != nil
}

// Initialize 在用户触发首次初始化时调用。
// dataDir 参数保留用于前端兼容，实际数据目录由平台决定。
func (a *App) Initialize(dataDir string) error {
	if err := config.Save(dataDir); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	a.initWithConfig(cfg)
	return nil
}

// initWithConfig 在配置加载成功后初始化所有运行时模块。
// 只有全部步骤成功才会将 a.cfg 设为非 nil，防止半初始化状态下 IsInitialized() 误报。
func (a *App) initWithConfig(cfg *config.AppConfig) {
	config.Set(cfg)

	// 1. 异步加载 ONNX 模型（不阻塞 GUI，尽早调用）
	rag.InitEmbedder(config.ModelsDir(), a.logger)

	// 2. 打开全局数据库
	db, err := storage.Open(config.GlobalDBPath(), a.logger)
	if err != nil {
		a.logger.Error("打开数据库失败", "err", err)
		return
	}
	a.db = db

	// 3. 自动建表
	if err := migrate.Run(db, a.logger); err != nil {
		a.logger.Error("数据库迁移失败", "err", err)
		return
	}

	// 4. 加载运行时配置
	settings, err := config.LoadSettings(db)
	if err != nil {
		a.logger.Error("加载设置失败", "err", err)
		return
	}
	a.settings = settings

	// 5. 注册操作日志钩子（失败降级：回滚功能不可用，其余正常）
	if err := storage.RegisterOplogHooks(db); err != nil {
		a.logger.Error("注册操作日志钩子失败，回滚功能将不可用", "err", err)
	}

	// 6. 创建所有领域 store
	a.novel = novel.NewStore(db, a.logger)
	a.preference = preference.NewStore(db, a.logger)
	a.setting = setting.NewStore(db, a.logger)
	a.chapter = chapter.NewStore(db, a.logger)
	a.character = character.NewStore(db, a.logger)
	a.session = session.NewStore(db, a.logger)
	a.timeline = timeline.NewStore(db, a.logger)
	a.storyarc = storyarc.NewStore(db, a.logger)
	a.location = location.NewStore(db, a.logger)
	a.reader = reader.NewStore(db, a.logger)
	a.turnCommit = rollback.NewStore(db, a.logger)
	a.writing = writing.NewStore(db, a.logger)
	s, err := skill.NewStore(a.logger, config.UserSkillsDir())
	if err != nil {
		a.logger.Error("初始化 skill store 失败", "err", err)
	} else {
		a.skill = s
		// 初始化远程 skill 市场服务（基于 skillStore 和 logger）
		a.remote = remote.NewService(a.skill, a.logger)
	}

	// 7. 初始化 MCP 工具注册表
	a.registry = mcp_tools.NewRegistry(a.logger)
	mcp_tools.RegisterAllTools(a.registry)

	// 8. 初始化 LLM 客户端
	userConfig, err := llm.LoadUserConfig(config.LLMConfigPath())
	if err != nil {
		a.logger.Warn("加载 LLM 配置失败，使用空配置", "err", err)
		userConfig = &llm.UserLLMConfig{}
	}
	providers := llm.Merge(llm.Builtin, userConfig)
	a.llmClient = llm.NewClient(providers, a.logger)

	// 9. 初始化审批服务
	a.approvals = approval.NewService(a.logger, a.settings.ApprovalMode)

	// 10. 创建 Agent 实例（全局复用）
	a.cancelMgr = agent.NewCancelManager()
	a.agent = agent.New(a.llmClient, a.registry, a.session, a.db, a.approvals, a.logger, a.skill, a.cancelMgr)

	// 10.5 初始化 style store（全局风格素材）
	a.style = style.NewStore(db, a.logger)

	// 11. 异步初始化向量存储和搜索服务（不阻塞 UI）
	go func() {
		emb, err := rag.GetEmbedder()
		svc := search.NewService(a.logger, a.character, a.location,
			a.timeline, a.storyarc, a.chapter, nil)
		a.searchService.Store(svc)
		a.agent.SetSearchService(svc)
		if err != nil {
			a.logger.Error("获取 Embedder 失败，向量检索不可用", "err", err)
			return
		}
		sqlDB, err := a.db.DB()
		if err != nil {
			a.logger.Error("获取底层 SQL DB 失败，向量检索不可用", "err", err)
			return
		}
		rag.InitVectorStore(sqlDB, emb, a.logger)
		a.vectorStore = rag.GetVectorStore()
		a.logger.Info("向量存储初始化完成")

		// 初始化搜索服务
		svc = search.NewService(a.logger, a.character, a.location,
			a.timeline, a.storyarc, a.chapter, a.vectorStore)
		a.searchService.Store(svc)
		a.agent.SetSearchService(svc)

		// 初始化刷新队列并启动
		rag.InitRefreshQueue(a.vectorStore, a.chapter, a.novel, a.logger)
		rag.GetRefreshQueue().Start()

		// 首次启动全量索引（已有向量则跳过）
		rebuildCtx := context.Background()
		if err := rag.GetRefreshQueue().RebuildAll(rebuildCtx); err != nil {
			a.logger.Error("全量向量索引失败", "err", err)
		}
	}()

	a.cfg = cfg
	a.logger.Info("应用初始化完成", "data_dir", config.DataDirPath())
}

package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v2/pkg/runtime"
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
	"github.com/sigpanic/goink/internal/volume"
	"github.com/sigpanic/goink/internal/writing"
)

// App 是 Wails 绑定的根对象。前端通过 window.go.main.App 调用其导出方法。
// 各领域方法按文件拆分（novel.go / chapter.go 等），均接收 *App。
type App struct {
	ctx    context.Context
	cancel context.CancelFunc
	logger *slog.Logger

	// 启动状态：让前端能区分"未配置 / 初始化中 / 就绪 / 失败"，而不是只拿到一个
	// 布尔量。startupMu 同时保护 startupState 与 frontendReady —— 两者会交叉读取
	// （setStartupState 读 frontendReady 决定是否推送，FrontendReady 读
	// startupState 决定补发什么），必须放在同一临界区，否则可能出现双方都判定
	// "对方未就绪"而漏掉一次推送。详见 startup_state.go。
	startupMu     sync.Mutex
	startupState  StartupState
	frontendReady bool

	// 关窗确认：初始化期间拦截一次关闭，等前端弹窗回传用户选择，见 quit_confirm.go。
	// 与启动状态共用 startupMu：OnBeforeClose 需要在同一临界区里同时读到阶段与标记。
	quitConfirmPending bool
	quitConfirmed      bool

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
	volume     *volume.Store
}

// New 创建 App 实例。初始化在 OnStartup 中完成。
func New(logger *slog.Logger) *App {
	return &App{
		logger: logger,
		startupState: StartupState{
			Phase:   PhaseInitializing,
			Version: 1,
		},
	}
}

// ── 生命周期 ──────────────────────────────────────────────

// OnStartup 在 Wails 窗口创建后调用，完成基础设施初始化。
//
// 注意 Wails 是在独立 goroutine 中调用本函数的，因此初始化与前端加载并发：
// 前端很可能在迁移跑完之前就挂载完毕。此时它靠 startup:state 事件与
// GetStartupState 得知"正在初始化"，而不是靠 IsInitialized() 的布尔量——后者
// 会把"初始化中"和"初始化失败"都误判为"未配置"。
func (a *App) OnStartup(ctx context.Context) {
	a.ctx, a.cancel = context.WithCancel(ctx)

	cfg, err := config.Load()
	if err != nil {
		if errors.Is(err, config.ErrNotInitialized) {
			// 首次启动，不自动初始化，等前端展示 InitView 再由用户手动触发
			a.setStartupState(PhaseUnconfigured, "")
			return
		}
		a.logger.Error("加载配置失败", "err", err)
		a.setStartupState(PhaseFailed, err.Error())
		return
	}
	_ = a.runInit(cfg)
}

// runInit 执行初始化并维护启动状态：先进入 initializing，成功置 ready，失败置
// failed 并带上原因。OnStartup / Initialize / RetryStartup 共用，保证三个入口
// 的状态语义一致。
func (a *App) runInit(cfg *config.AppConfig) error {
	if err := a.initWithConfig(cfg); err != nil {
		a.logger.Error("应用初始化失败", "err", err)
		a.setStartupState(PhaseFailed, err.Error())
		return err
	}
	a.setStartupState(PhaseReady, "")
	return nil
}

// OnBeforeClose 在窗口关闭前调用，返回 true 表示阻止关闭。
//
// 初始化进行中（含数据库迁移）时劝阻退出：迁移本身可断点续跑，但用户往往
// 不知道自己在等什么，中途退出会白白浪费一次启动。这里选择"劝阻 + 允许"而非
// 硬阻止——强杀进程无法拦截，硬阻止只会把用户逼向更粗暴的退出方式。
//
// Wails v2 的 MessageDialog 在 Windows/Linux 会忽略自定义按钮（只给系统按钮，
// 返回值恒为 "Ok"/"OK"），无法表达"继续等待 / 退出"这组选项，因此改由前端渲染
// 弹窗：本函数只做判定与登记，用户的最终选择经 ConfirmQuit / CancelQuit 回来。
func (a *App) OnBeforeClose(ctx context.Context) bool {
	a.startupMu.Lock()
	decision := decideClose(a.startupState.Phase, a.frontendReady, a.quitConfirmPending, a.quitConfirmed)
	if decision == quitConfirm {
		a.quitConfirmPending = true
	}
	a.startupMu.Unlock()

	switch decision {
	case quitConfirm:
		a.logger.Info("初始化进行中，请求前端确认是否退出")
		runtime.EventsEmit(ctx, quitConfirmEventName)
		return true
	case quitForce:
		a.logger.Info("确认期间再次请求关闭窗口，直接退出")
		return false
	default:
		return false
	}
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

// IsInitialized 报告核心运行时是否已就绪。
//
// 启动阶段的前端路由应使用 GetStartupState，而非这个兼容性布尔接口。
func (a *App) IsInitialized() bool {
	return a.cfg != nil
}

// Initialize 在用户触发首次初始化时调用：写入指针文件后执行初始化。
// dataDir 参数保留用于前端兼容，实际数据目录由平台决定。
func (a *App) Initialize(dataDir string) error {
	if err := a.beginInitialization(PhaseUnconfigured); err != nil {
		return err
	}
	if err := config.Save(dataDir); err != nil {
		saveErr := fmt.Errorf("保存配置失败: %w", err)
		a.setStartupState(PhaseUnconfigured, saveErr.Error())
		return saveErr
	}

	cfg, err := config.Load()
	if err != nil {
		// 回到 unconfigured 而非 failed：指针文件刚写入却读不回来，属于"配置还没
		// 建立起来"。留在设置页让用户重选目录，比进错误页更可恢复——错误页的重试
		// 会再读同一个文件，必然再次失败。
		loadErr := fmt.Errorf("加载配置失败: %w", err)
		a.setStartupState(PhaseUnconfigured, loadErr.Error())
		return loadErr
	}

	return a.runInit(cfg)
}

// RetryStartup 重新执行初始化，供前端在失败页点"重试"时调用。
//
// 与 Initialize 的区别：不写指针文件——配置已经存在，重试只是重新读取并初始化。
// 若复用 Initialize，会拿前端传来的路径覆盖用户已有配置。
func (a *App) RetryStartup() error {
	if err := a.beginInitialization(PhaseFailed); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		if errors.Is(err, config.ErrNotInitialized) {
			// 指针文件在运行期被删除：回到首次配置页。否则用户会停在错误页，而
			// 重试读的是同一个已不存在的文件，永远失败。
			loadErr := fmt.Errorf("加载配置失败: %w", err)
			a.setStartupState(PhaseUnconfigured, loadErr.Error())
			return loadErr
		}
		a.setStartupState(PhaseFailed, err.Error())
		return fmt.Errorf("加载配置失败: %w", err)
	}
	return a.runInit(cfg)
}

// initWithConfig 在配置加载成功后初始化所有运行时模块。
// 只有全部步骤成功才会将 a.cfg 设为非 nil，防止半初始化状态下 IsInitialized() 误报。
// 返回 error 让调用方（OnStartup/Initialize）能感知失败并通知前端。
func (a *App) initWithConfig(cfg *config.AppConfig) error {
	config.Set(cfg)

	// 1. 异步加载 ONNX 模型（不阻塞 GUI，尽早调用）
	rag.InitEmbedder(config.ModelsDir(), a.logger)

	// 2. 打开全局数据库
	db, err := storage.Open(config.GlobalDBPath(), a.logger)
	if err != nil {
		a.logger.Error("打开数据库失败", "err", err)
		return fmt.Errorf("打开数据库失败: %w", err)
	}
	initSucceeded := false
	defer func() {
		if !initSucceeded {
			if closeErr := storage.Close(db); closeErr != nil {
				a.logger.Warn("关闭失败初始化的数据库连接失败", "err", closeErr)
			}
		}
	}()

	// 3. 自动建表
	if err := migrate.Run(db, a.logger); err != nil {
		a.logger.Error("数据库迁移失败", "err", err)
		return fmt.Errorf("数据库迁移失败: %w", err)
	}

	// 4. 加载运行时配置
	settings, err := config.LoadSettings(db)
	if err != nil {
		a.logger.Error("加载设置失败", "err", err)
		return fmt.Errorf("加载设置失败: %w", err)
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
	a.volume = volume.NewStore(db, a.logger)
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
	a.agent = agent.New(a.llmClient, a.registry, a.session, a.chapter, db, a.approvals, a.logger, a.skill, a.cancelMgr)

	// 10.5 初始化 style store（全局风格素材）
	a.style = style.NewStore(db, a.logger)
	a.db = db

	// 11. 异步初始化向量存储和搜索服务（不阻塞 UI）
	go func() {
		emb, err := rag.GetEmbedder()
		svc := search.NewService(a.logger, a.character, a.location,
			a.timeline, a.storyarc, a.chapter, a.reader, a.preference, a.setting, nil)
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
			a.timeline, a.storyarc, a.chapter, a.reader, a.preference, a.setting, a.vectorStore)
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
	initSucceeded = true
	return nil
}

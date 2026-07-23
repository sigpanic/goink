package app

import (
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	imp "github.com/sigpanic/goink/internal/import"
)

// ImportNovelResult 是导入小说的返回结果。
type ImportNovelResult = imp.ImportResult

// ImportProgress 是导入过程的前端进度事件。
type ImportProgress struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
	Current int    `json:"current"`
	Total   int    `json:"total"`
	Percent int    `json:"percent"`
	NovelID int64  `json:"novel_id,omitempty"`
}

// ImportNovelInput 是导入小说的入参。
type ImportNovelInput struct {
	FilePath string `json:"file_path"`
}

// ImportNovel 从文件导入一部小说：解析文件 → 创建 Novel → 写入章节文件 → Git 提交。
func (a *App) ImportNovel(input ImportNovelInput) (*ImportNovelResult, error) {
	return imp.Import(a.ctx, a.logger, a.novel.DB, input.FilePath, a.settings.GitName, a.settings.GitEmail, a.emitImportProgress)
}

func (a *App) emitImportProgress(stage, message string, current, total, percent int, novelID int64) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "import:progress", ImportProgress{
		Stage:   stage,
		Message: message,
		Current: current,
		Total:   total,
		Percent: percent,
		NovelID: novelID,
	})
}

// PickAndImportNovel 打开文件选择对话框，然后导入选中的小说文件。
func (a *App) PickAndImportNovel() (*ImportNovelResult, error) {
	filePath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "导入小说",
		Filters: []runtime.FileFilter{
			{DisplayName: "电子书 (*.epub, *.txt, *.md)", Pattern: "*.epub;*.txt;*.md;*.markdown"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("选择文件失败: %w", err)
	}
	if filePath == "" {
		return nil, nil // 用户取消
	}

	return a.ImportNovel(ImportNovelInput{FilePath: filePath})
}

// ImportWithLLMInput 是 AI 辅助导入小说的入参。
type ImportWithLLMInput struct {
	FilePath     string `json:"file_path"`
	ProviderName string `json:"provider_name"`
	ModelID      string `json:"model_id"`
}

// ImportWithLLM 使用 LLM 分析章节格式后导入小说。
// 当 ImportNovel 返回 NeedsLLM=true 时，前端调用此方法。
// 流程：LLM 生成正则 → 全文分割 → 校验 → 导入。
func (a *App) ImportWithLLM(input ImportWithLLMInput) (*ImportNovelResult, error) {
	// 1. LLM 分析 + 分割（内含 isReasonableChapterCount 校验）
	result, err := imp.AnalyzeWithLLM(a.ctx, input.FilePath, input.ProviderName, input.ModelID, a.llmClient.GenerateText)
	if err != nil {
		return nil, err
	}

	// 2. 直接导入
	return imp.ImportWithResult(a.ctx, a.logger, a.novel.DB, result, input.FilePath, a.settings.GitName, a.settings.GitEmail, a.emitImportProgress)
}

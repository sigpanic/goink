package app

import (
	"context"
	"fmt"

	"github.com/sigpanic/goink/internal/agent"
	"github.com/sigpanic/goink/internal/storage"
	"github.com/sigpanic/goink/internal/style"
	"github.com/sigpanic/goink/internal/text"
)

// ListStyleSamplesInput 是列出风格素材的入参。
type ListStyleSamplesInput struct {
	NovelID int64  `json:"novel_id"`
	Page    int    `json:"page"`
	Size    int    `json:"size"`
	Search  string `json:"search"`
}

// ListStyleSamples 分页列出风格素材。
func (a *App) ListStyleSamples(input ListStyleSamplesInput) (*storage.PageResult[style.Sample], error) {
	result, err := a.style.List(a.ctx, style.ListOptions{
		PageParams: storage.PageParams{Page: input.Page, Size: input.Size},
		NovelID:    input.NovelID,
		Search:     input.Search,
	})
	if err != nil {
		return nil, fmt.Errorf("list style samples: %w", err)
	}
	return result, nil
}

// CreateStyleSampleInput 是创建风格素材的入参。
type CreateStyleSampleInput struct {
	NovelID  int64    `json:"novel_id"`
	IsGlobal bool     `json:"is_global"`
	Name     string   `json:"name"`
	Content  string   `json:"content"`
	Tags     []string `json:"tags"`
}

// CreateStyleSample 创建一条风格素材。
func (a *App) CreateStyleSample(input CreateStyleSampleInput) (*style.Sample, error) {
	sample := &style.Sample{
		NovelID:   input.NovelID,
		IsGlobal:  input.IsGlobal,
		Name:      input.Name,
		Content:   input.Content,
		Preview:   style.TruncatePreview(input.Content),
		Tags:      style.StringSlice(input.Tags),
		WordCount: text.ComputeStats(input.Content).WordCount,
	}
	if err := a.style.DB.WithContext(a.ctx).Create(sample).Error; err != nil {
		return nil, fmt.Errorf("create style sample: %w", err)
	}
	return sample, nil
}

// UpdateStyleSampleInput 是更新风格素材的入参。
type UpdateStyleSampleInput struct {
	ID       int64    `json:"id"`
	Name     string   `json:"name"`
	Content  string   `json:"content"`
	Tags     []string `json:"tags"`
	IsGlobal bool     `json:"is_global"`
	NovelID  int64    `json:"novel_id"`
}

// UpdateStyleSample 更新一条风格素材。
func (a *App) UpdateStyleSample(input UpdateStyleSampleInput) (*style.Sample, error) {
	var sample style.Sample
	if err := a.style.DB.WithContext(a.ctx).First(&sample, input.ID).Error; err != nil {
		return nil, fmt.Errorf("update style sample: %w", err)
	}
	sample.Name = input.Name
	sample.Content = input.Content
	sample.Preview = style.TruncatePreview(input.Content)
	sample.Tags = style.StringSlice(input.Tags)
	sample.WordCount = text.ComputeStats(input.Content).WordCount
	sample.IsGlobal = input.IsGlobal
	sample.NovelID = input.NovelID
	if err := a.style.DB.WithContext(a.ctx).Save(&sample).Error; err != nil {
		return nil, fmt.Errorf("update style sample: %w", err)
	}
	return &sample, nil
}

// GetStyleSample 获取单条风格素材的完整内容。
func (a *App) GetStyleSample(id int64) (*style.Sample, error) {
	var sample style.Sample
	if err := a.style.DB.WithContext(a.ctx).First(&sample, id).Error; err != nil {
		return nil, fmt.Errorf("get style sample: %w", err)
	}
	return &sample, nil
}

// DeleteStyleSampleInput 是删除风格素材的入参。
type DeleteStyleSampleInput struct {
	ID int64 `json:"id"`
}

// DeleteStyleSample 删除一条风格素材。
func (a *App) DeleteStyleSample(input DeleteStyleSampleInput) error {
	if err := a.style.DB.WithContext(a.ctx).Delete(&style.Sample{}, input.ID).Error; err != nil {
		return fmt.Errorf("delete style sample: %w", err)
	}
	return nil
}

// ComputeStyleStatsInput 是计算风格统计的入参。
type ComputeStyleStatsInput struct {
	SampleIDs []int64 `json:"sample_ids"`
}

// ComputeStyleStats 对指定的风格素材进行确定性文本统计。
func (a *App) ComputeStyleStats(input ComputeStyleStatsInput) (*style.Stats, error) {
	samples, err := a.style.GetByIDs(a.ctx, input.SampleIDs)
	if err != nil {
		return nil, fmt.Errorf("compute style stats: %w", err)
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("没有选中的素材")
	}
	stats := style.ComputeStats(samples)
	return &stats, nil
}

// ExtractStyleInput 是风格提取的入参。
type ExtractStyleInput struct {
	TaskID          string  `json:"task_id"`
	SampleIDs       []int64 `json:"sample_ids"`
	ProviderName    string  `json:"provider_name"`
	ModelID         string  `json:"model_id"`
	ReasoningEffort string  `json:"reasoning_effort"`
}

// ExtractStyle 从选中的素材中提取写作风格，生成仿写 skill。
func (a *App) ExtractStyle(input ExtractStyleInput) (*style.ExtractResult, error) {
	if a.llmClient == nil {
		return nil, fmt.Errorf("LLM 客户端未初始化")
	}
	if len(input.SampleIDs) == 0 {
		return nil, fmt.Errorf("请选择至少一段素材")
	}

	// 批量加载选中素材
	samples, err := a.style.GetByIDs(a.ctx, input.SampleIDs)
	if err != nil {
		return nil, fmt.Errorf("加载素材失败: %w", err)
	}

	// 取消逻辑由 app 层管理
	key := agent.CancelPrefixStyle + input.TaskID
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelMgr.Cancel(key)
	a.cancelMgr.Register(key, cancel)
	defer func() {
		if ctx.Err() == nil {
			a.cancelMgr.Unregister(key)
		}
	}()

	return style.Extract(ctx, a.llmClient, samples,
		input.ProviderName, input.ModelID, input.ReasoningEffort)
}

// CancelExtract 取消指定 taskID 的风格提取任务。
func (a *App) CancelExtract(taskID string) {
	a.cancelMgr.Cancel(agent.CancelPrefixStyle + taskID)
}

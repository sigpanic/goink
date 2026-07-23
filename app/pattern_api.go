package app

import (
	"context"
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/sigpanic/goink/internal/agent"
	"github.com/sigpanic/goink/internal/pattern"
)

type ExtractPatternInput = pattern.ExtractPatternInput
type ExtractPatternResult = pattern.ExtractPatternResult
type ExtractPatternProgress = pattern.Progress

// ExtractPattern 从指定小说中提取叙事套路模式
func (a *App) ExtractPattern(input ExtractPatternInput) (*ExtractPatternResult, error) {
	if a.llmClient == nil {
		return nil, fmt.Errorf("LLM 客户端未初始化")
	}
	if a.chapter == nil {
		return nil, fmt.Errorf("章节存储未初始化")
	}

	ctx, cancel := context.WithCancel(a.ctx)
	defer cancel()

	key := fmt.Sprintf("%s%d", agent.CancelPrefixPattern, input.NovelID)
	a.cancelMgr.Cancel(key)
	a.cancelMgr.Register(key, cancel)
	defer func() {
		if ctx.Err() == nil {
			a.cancelMgr.Unregister(key)
		}
	}()

	extractor := &pattern.Extractor{
		Chapters:  a.chapter,
		LLMClient: a.llmClient,
		DB:        a.db,
		Progress:  a.emitExtractPatternProgress,
	}
	result, err := extractor.Extract(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("提取模式失败: %w", err)
	}
	return result, nil
}

// CancelExtractPattern 取消正在进行的小说模式提取
func (a *App) CancelExtractPattern(novelID int64) {
	a.cancelMgr.Cancel(fmt.Sprintf("%s%d", agent.CancelPrefixPattern, novelID))
}

func (a *App) emitExtractPatternProgress(progress pattern.Progress) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "pattern:progress", progress)
}

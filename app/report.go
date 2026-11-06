package app

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/sigpanic/goink/internal/apperr"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/report"
)

// MarkAnnualReportPrompted 按报告年份记录已展示的提醒，跨年一月仍属于上一份报告。
func (a *App) MarkAnnualReportPrompted(year int) error {
	if year < 2026 || year > time.Now().Year() {
		return fmt.Errorf("report: prompt year must be between 2026 and the current year")
	}
	return a.updateSettings(func(s *config.AppSettings) {
		s.AnnualReportPromptYear = max(s.AnnualReportPromptYear, year)
	})
}

// GetAnnualReport 复用领域报告结构，年份校验及统计编排由报告服务负责。
func (a *App) GetAnnualReport(year int) *apperr.Result[report.AnnualReport] {
	if a.reportService == nil {
		return apperr.Err[report.AnnualReport](errors.New("report: service is not ready"))
	}
	result, err := a.reportService.GetAnnual(a.ctx, year)
	if err != nil {
		if errors.Is(err, report.ErrInvalidYear) {
			err = &apperr.BusinessError{CodeVal: report.CodeInvalidYear, Msg: "report: year must be between 1 and the current year", Cause: err}
		}
		return apperr.Err[report.AnnualReport](err)
	}
	return apperr.Ok(*result)
}

type SaveAnnualReportImageInput struct {
	Year      int    `json:"year"`
	PNGBase64 string `json:"png_base64"`
}

// SaveAnnualReportImage 只保存用户预览的 PNG；返回 false 表示用户取消保存。
func (a *App) SaveAnnualReportImage(input SaveAnnualReportImageInput) *apperr.Result[bool] {
	return saveAnnualReportImage(input, func(options runtime.SaveDialogOptions) (string, error) {
		return runtime.SaveFileDialog(a.ctx, options)
	})
}

func saveAnnualReportImage(input SaveAnnualReportImageInput, choosePath func(runtime.SaveDialogOptions) (string, error)) *apperr.Result[bool] {
	if input.Year < 1 || input.Year > time.Now().Year() {
		return apperr.Err[bool](&apperr.BusinessError{CodeVal: report.CodeInvalidYear, Msg: "report: year must be between 1 and the current year"})
	}
	data, err := base64.StdEncoding.DecodeString(input.PNGBase64)
	if err != nil {
		return apperr.Err[bool](&apperr.BusinessError{CodeVal: report.CodeInvalidImage, Msg: "report: decode poster base64", Cause: err})
	}
	if _, err := png.DecodeConfig(bytes.NewReader(data)); err != nil {
		return apperr.Err[bool](&apperr.BusinessError{CodeVal: report.CodeInvalidImage, Msg: "report: decode poster PNG header", Cause: err})
	}
	path, err := choosePath(runtime.SaveDialogOptions{
		DefaultFilename:      fmt.Sprintf("Goink-%d.png", input.Year),
		Title:                "保存年度报告图片",
		Filters:              []runtime.FileFilter{{DisplayName: "PNG (*.png)", Pattern: "*.png"}},
		CanCreateDirectories: true,
	})
	if err != nil {
		return apperr.Err[bool](fmt.Errorf("report: save dialog: %w", err))
	}
	if path == "" {
		return apperr.Ok(false)
	}
	if !strings.EqualFold(filepath.Ext(path), ".png") {
		path += ".png"
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return apperr.Err[bool](fmt.Errorf("report: save image: %w", err))
	}
	return apperr.Ok(true)
}

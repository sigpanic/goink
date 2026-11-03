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
	"github.com/sigpanic/goink/internal/report"
)

// GetAnnualReport 复用领域报告结构，年份校验及统计编排由报告服务负责。
func (a *App) GetAnnualReport(year int) *apperr.Result[report.AnnualReport] {
	if a.reportService == nil {
		return apperr.Err[report.AnnualReport](errors.New("年度报告服务尚未就绪"))
	}
	result, err := a.reportService.GetAnnual(a.ctx, year)
	if err != nil {
		if errors.Is(err, report.ErrInvalidYear) {
			err = &apperr.BusinessError{CodeVal: apperr.CodeInvalid, Msg: "报告年份必须介于 1 和当前年份之间", Cause: err}
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
		return apperr.Err[bool](apperr.NewInvalid("报告年份无效"))
	}
	data, err := base64.StdEncoding.DecodeString(input.PNGBase64)
	if err != nil {
		return apperr.Err[bool](apperr.NewInvalid("分享图片编码无效"))
	}
	if _, err := png.DecodeConfig(bytes.NewReader(data)); err != nil {
		return apperr.Err[bool](apperr.NewInvalid("分享图片不是有效的 PNG"))
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

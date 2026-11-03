package app

import (
	"errors"

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

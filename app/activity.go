package app

import "github.com/sigpanic/goink/internal/activity"

// RecordCreativeActivity 累计近似活跃秒数，单次最多计一分钟；统计失败不向前端返回错误。
func (a *App) RecordCreativeActivity(seconds int64) {
	if seconds <= 0 || a.activity == nil {
		return
	}
	seconds = min(seconds, 60)
	a.activity.AddActivity(a.ctx, activity.ActivityDelta{CreativeSeconds: seconds})
}

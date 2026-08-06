package storage

// PageResult 泛型分页响应，匹配 Python PageResponse 的语义。
type PageResult[T any] struct {
	Items      []T   `json:"items"`
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	Size       int   `json:"size"`
	TotalPages int   `json:"total_pages"`
}

// PageParams 是所有 List 方法的分页参数，零值即可直接使用（Page=0 时 normalize 为 1）。
type PageParams struct {
	Page int `json:"page"`
	Size int `json:"size"`
}

// Normalize 归一化分页参数，语义对齐 GORM：
//   - Size >= 0 原样透传（0=Limit(0) 返回 0 条，快速失败；>0 正常分页）
//   - Size < 0 归一化为 -1（GORM Limit(-1) 取消限制，表示全量），并强制 Page=1 保证 offset=0
func (p *PageParams) Normalize() *PageParams {
	if p.Size < 0 {
		p.Size = -1
		p.Page = 1 // 全量强制首页，offset 恒 0
	} else if p.Page < 1 {
		p.Page = 1
	}
	return p
}

// Offset 计算分页偏移量。Size=-1（全量）时 Page 已被 Normalize 强制为 1，返回 0。
func (p PageParams) Offset() int {
	return (p.Page - 1) * p.Size
}

// NewPageResult 根据 total/size 自动计算 TotalPages。
func NewPageResult[T any](items []T, total int64, page, size int) *PageResult[T] {
	tp := 0
	if size > 0 {
		tp = int(total) / size
		if int(total)%size != 0 {
			tp++
		}
	}
	return &PageResult[T]{
		Items:      items,
		Total:      total,
		Page:       page,
		Size:       size,
		TotalPages: tp,
	}
}

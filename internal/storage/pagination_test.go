package storage

import "testing"

// TestNewPageResult_NilItems 验证 nil 归一化为空切片，避免 JSON 序列化出 null。
func TestNewPageResult_NilItems(t *testing.T) {
	r := NewPageResult[int](nil, 0, 1, -1)
	if r.Items == nil {
		t.Error("expected non-nil empty Items, got nil")
	}
	if len(r.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(r.Items))
	}
}

// TestNewPageResult_TotalPages 验证各 size 档位的 TotalPages 语义。
func TestNewPageResult_TotalPages(t *testing.T) {
	cases := []struct {
		name  string
		total int64
		size  int
		want  int
	}{
		{"full with data", 5, -1, 1},
		{"full empty", 0, -1, 0},
		{"zero size", 5, 0, 0},
		{"exact page", 20, 20, 1},
		{"remainder page", 45, 20, 3},
		{"empty paginated", 0, 20, 0},
		{"single item", 1, 20, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := NewPageResult[int]([]int{1, 2, 3}, c.total, 1, c.size)
			if r.TotalPages != c.want {
				t.Errorf("size=%d total=%d: TotalPages = %d, want %d", c.size, c.total, r.TotalPages, c.want)
			}
		})
	}
}

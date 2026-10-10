package text

import (
	"strings"
	"testing"
)

func TestComputeWordChanges(t *testing.T) {
	for _, tc := range []struct {
		name, previous, current string
		added, deleted          int
	}{
		{"empty", "", "", 0, 0},
		{"first save", "", "主角 ZhangSan 走进了 room。", 7, 0},
		{"delete all", "中文 hello", "", 0, 3},
		{"unchanged", "主角走进房间", "主角走进房间", 0, 0},
		{"equal length rewrite", "主角走进房间", "主角冲出房间", 2, 2},
		{"deletion and addition", strings.Repeat("前", 600) + strings.Repeat("甲", 200), strings.Repeat("前", 600) + strings.Repeat("乙", 250), 250, 200},
		{"english replacement", "hello world", "help world", 1, 1},
		{"english split", "hello", "hel lo", 2, 1},
		{"contraction", "I can't go", "I can go", 1, 1},
		{"punctuation only", "主角 hello，世界！123", "主角\nhello 世界？456", 0, 0},
		{"unicode Han", "𠀀人", "𠀀天", 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeWordChanges(tc.previous, tc.current)
			if got.Added != tc.added || got.Deleted != tc.deleted {
				t.Fatalf("changes = %+v, want added %d, deleted %d", got, tc.added, tc.deleted)
			}
			if got.Added-got.Deleted != ComputeStats(tc.current).WordCount-ComputeStats(tc.previous).WordCount {
				t.Fatal("diff net change must match word count change")
			}
		})
	}
}

func BenchmarkComputeWordChanges(b *testing.B) {
	previous := strings.Repeat("主角走进房间。\n", 800)
	current := strings.Replace(previous, "走进", "冲出", 1)
	b.ReportAllocs()
	for b.Loop() {
		ComputeWordChanges(previous, current)
	}
}

//go:build cgo

package mcp_tools

import "testing"

// TestParseChapterRef 章节正文路径解析：扁平主格式、两级容错格式、new 通道、非法形态。
func TestParseChapterRef(t *testing.T) {
	cases := []struct {
		name string
		path string
		want chapterRef
		ok   bool
	}{
		{"flat_id", "chapters/id_5.md", chapterRef{ID: 5}, true},
		{"flat_big_id", "chapters/id_999999999999.md", chapterRef{ID: 999999999999}, true},
		{"two_level_id", "chapters/3/id_5.md", chapterRef{ID: 5, Vid: 3}, true},
		{"two_level_new", "chapters/3/new.md", chapterRef{Vid: 3, IsNew: true}, true},
		{"flat_new", "chapters/new.md", chapterRef{Vid: 0, IsNew: true}, true},
		{"zero_id", "chapters/id_0.md", chapterRef{ID: 0}, true},

		// 超出 int64 范围的数字段按非法路径处理，不得静默归零
		{"id_overflow", "chapters/id_99999999999999999999.md", chapterRef{}, false},
		{"vid_overflow", "chapters/99999999999999999999/id_1.md", chapterRef{}, false},

		// 旧 num 命名空间必须被拒绝（与 id_ 命名空间结构性隔离）
		{"legacy_num", "chapters/001.md", chapterRef{}, false},
		{"missing_id_prefix", "chapters/5.md", chapterRef{}, false},
		{"wrong_ext", "chapters/id_5.txt", chapterRef{}, false},
		{"extra_level", "chapters/3/4/id_5.md", chapterRef{}, false},
		{"empty_vid", "chapters//id_5.md", chapterRef{}, false},
		{"non_numeric", "chapters/id_abc.md", chapterRef{}, false},
		{"trailing_garbage", "chapters/id_5.md.bak", chapterRef{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := parseChapterRef(c.path)
			if ok != c.ok {
				t.Fatalf("parseChapterRef(%q) ok = %v, want %v", c.path, ok, c.ok)
			}
			if ok && got != c.want {
				t.Errorf("parseChapterRef(%q) = %+v, want %+v", c.path, got, c.want)
			}
		})
	}
}

// TestParseOutlineRef 大纲路径与章节路径同构。
func TestParseOutlineRef(t *testing.T) {
	cases := []struct {
		name string
		path string
		want chapterRef
		ok   bool
	}{
		{"flat_id", "outlines/id_12.md", chapterRef{ID: 12}, true},
		{"two_level_id", "outlines/7/id_12.md", chapterRef{ID: 12, Vid: 7}, true},
		{"two_level_new", "outlines/7/new.md", chapterRef{Vid: 7, IsNew: true}, true},
		{"flat_new", "outlines/new.md", chapterRef{Vid: 0, IsNew: true}, true},
		{"legacy_num", "outlines/012.md", chapterRef{}, false},
		{"chapter_prefix_rejected", "chapters/id_1.md", chapterRef{}, false},
		{"id_overflow", "outlines/id_99999999999999999999.md", chapterRef{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := parseOutlineRef(c.path)
			if ok != c.ok {
				t.Fatalf("parseOutlineRef(%q) ok = %v, want %v", c.path, ok, c.ok)
			}
			if ok && got != c.want {
				t.Errorf("parseOutlineRef(%q) = %+v, want %+v", c.path, got, c.want)
			}
		})
	}
}

// TestParseRWPath 章节类/大纲类分流。
func TestParseRWPath(t *testing.T) {
	if ref, isOutline, ok := parseRWPath("chapters/id_1.md"); !ok || isOutline || ref.ID != 1 {
		t.Errorf("chapter path: got ref=%+v isOutline=%v ok=%v", ref, isOutline, ok)
	}
	if ref, isOutline, ok := parseRWPath("outlines/2/id_1.md"); !ok || !isOutline || ref.Vid != 2 {
		t.Errorf("outline path: got ref=%+v isOutline=%v ok=%v", ref, isOutline, ok)
	}
	if _, _, ok := parseRWPath("goink.md"); ok {
		t.Error("goink.md should not parse as chapter-like")
	}
	if _, _, ok := parseRWPath("skills/foo.md"); ok {
		t.Error("skill path should not parse as chapter-like")
	}
}

// TestValidPath edit/read 支持的全部路径形态。
func TestValidPath(t *testing.T) {
	valid := []string{
		"chapters/id_1.md",
		"chapters/3/id_1.md",
		"chapters/3/new.md",
		"outlines/id_1.md",
		"outlines/3/new.md",
		"goink.md",
		"skills/my-skill.md",
		"~/.goink/skills/my-skill.md",
	}
	for _, p := range valid {
		if !validPath(p) {
			t.Errorf("validPath(%q) = false, want true", p)
		}
	}

	invalid := []string{
		"chapters/001.md",
		"outlines/001.md",
		"chapters/new",        // 缺扩展名
		"chapters/id_1.md/",   // 尾部斜杠
		"plans/weekly.md",     // 不支持的目录
		"volumes/1.md",        // 卷纲不经过 edit/read
		"skills/",             // 缺文件名
		"skills/a/b.md",       // 多级技能名
		"/etc/passwd",         // 绝对路径（builtin 前缀之外）
		"~/.goink/other/x.md", // 用户目录下非 skills 路径
	}
	for _, p := range invalid {
		if validPath(p) {
			t.Errorf("validPath(%q) = true, want false", p)
		}
	}
}

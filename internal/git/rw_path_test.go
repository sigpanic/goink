package git

import "testing"

func TestParseChapterLikePath(t *testing.T) {
	cases := []struct {
		name string
		path string
		want ChapterPathRef
		ok   bool
	}{
		{"flat chapter", "chapters/id_5.md", ChapterPathRef{ID: 5}, true},
		{"flat large ID", "chapters/id_999999999999.md", ChapterPathRef{ID: 999999999999}, true},
		{"volume chapter", "chapters/3/id_5.md", ChapterPathRef{ID: 5, VolumeID: 3}, true},
		{"new chapter", "chapters/3/new.md", ChapterPathRef{VolumeID: 3, IsNew: true}, true},
		{"new unassigned chapter", "chapters/new.md", ChapterPathRef{IsNew: true}, true},
		{"zero ID", "chapters/id_0.md", ChapterPathRef{}, true},
		{"flat outline", "outlines/id_12.md", ChapterPathRef{ID: 12, IsOutline: true}, true},
		{"volume outline", "outlines/7/id_12.md", ChapterPathRef{ID: 12, VolumeID: 7, IsOutline: true}, true},
		{"new outline", "outlines/7/new.md", ChapterPathRef{VolumeID: 7, IsOutline: true, IsNew: true}, true},

		{"legacy chapter number", "chapters/001.md", ChapterPathRef{}, false},
		{"legacy outline number", "outlines/012.md", ChapterPathRef{}, false},
		{"ID overflow", "chapters/id_99999999999999999999.md", ChapterPathRef{}, false},
		{"volume ID overflow", "chapters/99999999999999999999/id_1.md", ChapterPathRef{}, false},
		{"missing ID prefix", "chapters/5.md", ChapterPathRef{}, false},
		{"wrong extension", "chapters/id_5.txt", ChapterPathRef{}, false},
		{"extra level", "chapters/3/4/id_5.md", ChapterPathRef{}, false},
		{"empty volume", "chapters//id_5.md", ChapterPathRef{}, false},
		{"non-numeric ID", "chapters/id_abc.md", ChapterPathRef{}, false},
		{"trailing garbage", "chapters/id_5.md.bak", ChapterPathRef{}, false},
		{"plain file", "goink.md", ChapterPathRef{}, false},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseChapterLikePath(tt.path)
			if ok != tt.ok {
				t.Fatalf("ParseChapterLikePath(%q) ok = %t, want %t", tt.path, ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("ParseChapterLikePath(%q) = %+v, want %+v", tt.path, got, tt.want)
			}
		})
	}
}

func TestParseVolumePath(t *testing.T) {
	cases := []struct {
		name string
		path string
		want int64
		ok   bool
	}{
		{"volume outline", "volumes/12.md", 12, true},
		{"large ID", "volumes/999999999999.md", 999999999999, true},
		{"zero ID", "volumes/0.md", 0, false},
		{"leading zero", "volumes/012.md", 0, false},
		{"negative ID", "volumes/-1.md", 0, false},
		{"chapter path", "chapters/id_12.md", 0, false},
		{"wrong extension", "volumes/12.txt", 0, false},
		{"overflow", "volumes/99999999999999999999.md", 0, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseVolumePath(tt.path)
			if ok != tt.ok || got != tt.want {
				t.Errorf("ParseVolumePath(%q) = (%d, %t), want (%d, %t)", tt.path, got, ok, tt.want, tt.ok)
			}
		})
	}
}

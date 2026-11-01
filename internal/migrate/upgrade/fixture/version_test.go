package fixture

import (
	"reflect"
	"testing"
)

func TestHistoricalSelection(t *testing.T) {
	m := Manifest{Fixtures: []Fixture{
		{Tag: "v1.0.0", Origin: "fresh"},
		{Tag: "v1.6.0", Origin: "fresh"},
		{Tag: "v1.6.0", Origin: "from-v1.0.0"},
		{Tag: "v1.6.1", Origin: "fresh"},
		{Tag: "v1.7.0", Origin: "fresh"},
		{Tag: "v1.10.0", Origin: "fresh"},
		{Tag: "v2.0.0", Origin: "fresh"},
	}}
	for _, tc := range []struct {
		target string
		count  int
	}{
		{"v1.0.0", 0},
		{"v1.6.0", 1},
		{"v1.6.1", 3},
		{"v1.7.1", 5},
		{"v1.10.0", 5},
		{"v2.0.0", 6},
		{"", 7},
		{"ci-upgrade", 7},
	} {
		t.Run(tc.target, func(t *testing.T) {
			got := m.Historical(tc.target)
			if len(got) != tc.count {
				t.Fatalf("selected %d samples, want %d", len(got), tc.count)
			}
			for i, sample := range got {
				if !reflect.DeepEqual(sample, m.Fixtures[i]) {
					t.Fatalf("selection changed sample: %+v", sample)
				}
			}
		})
	}
}

func TestUnsupportedReleaseTags(t *testing.T) {
	for _, tag := range []string{"v0.9.0", "v1.0", "v1.6.0-rc.1", "v01.0.0", "dev", "v18446744073709551616.0.0"} {
		if SupportedTag(tag) || BeforeTarget(tag, "") {
			t.Errorf("unsupported tag was selected: %q", tag)
		}
	}
}

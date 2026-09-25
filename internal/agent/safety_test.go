package agent

import (
	"testing"
)

func TestIsStuckLoop_NotEnoughTurns(t *testing.T) {
	if isStuckLoop([]string{"a", "b"}, nil, 2) {
		t.Error("should not detect loop with < 4 patterns")
	}
	if isStuckLoop([]string{"a", "b", "c"}, nil, 3) {
		t.Error("should not detect loop with loopCount < 4")
	}
}

func TestIsStuckLoop_TooManyPatterns(t *testing.T) {
	patterns := []string{"a", "b", "c", "d"} // 4 patterns, 4 unique → > 2 uniq
	if isStuckLoop(patterns, nil, 4) {
		t.Error("should not detect loop with > 2 unique patterns")
	}
}

func TestIsStuckLoop_RepeatingPatternsReadOnly(t *testing.T) {
	patterns := []string{"a", "b", "a", "b"} // 2 unique, 4 turns
	outputs := []toolOutput{
		{name: "read", id: "1"},
		{name: "search_story_memory", id: "2"},
	}
	if !isStuckLoop(patterns, outputs, 4) {
		t.Error("should detect loop: 2 repeating patterns + all read-only")
	}
}

func TestIsStuckLoop_HasWriteTool(t *testing.T) {
	patterns := []string{"a", "b", "a", "b"}
	outputs := []toolOutput{
		{name: "read", id: "1"},
		{name: "edit", id: "2"}, // write tool, not in readOnlyTools
	}
	if isStuckLoop(patterns, outputs, 4) {
		t.Error("should not detect loop when a write tool is present")
	}
}

func TestIsStuckLoop_OnlyReadsOldTurn(t *testing.T) {
	// loopCount < 4 时不检测
	patterns := []string{"a", "b", "a", "b"}
	outputs := []toolOutput{
		{name: "read", id: "1"},
	}
	if isStuckLoop(patterns, outputs, 3) {
		t.Error("should not detect loop when loopCount < 4")
	}
}

func TestToolPattern_Single(t *testing.T) {
	outputs := []toolOutput{
		{name: "read", id: "1", rawArgs: []byte(`{"path":"chapters/id_1.md"}`)},
	}
	result := toolPattern(outputs)
	expected := "read:{\"path\":\"chapters/id_1.md\"}"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestToolPattern_Sorted(t *testing.T) {
	outputs := []toolOutput{
		{name: "get_characters", id: "2", rawArgs: []byte(`{}`)},
		{name: "read", id: "1", rawArgs: []byte(`{"path":"chapters/id_3.md"}`)},
	}
	result := toolPattern(outputs)
	// 排序后 get_characters 在前
	expected := "get_characters:{}|read:{\"path\":\"chapters/id_3.md\"}"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestToolPattern_TruncatesLongArgs(t *testing.T) {
	long := ""
	for i := 0; i < 150; i++ {
		long += "x"
	}
	outputs := []toolOutput{
		{name: "read", id: "1", rawArgs: []byte(long)},
	}
	result := toolPattern(outputs)
	if len(result) > 105 { // "read:" = 5 + 100 max
		t.Errorf("pattern too long: %d chars", len(result))
	}
}

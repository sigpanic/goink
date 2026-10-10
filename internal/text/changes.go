package text

import (
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/sergi/go-diff/diffmatchpatch"
)

type WordChanges struct {
	Added   int
	Deleted int
}

// ComputeWordChanges 比较保存前后的中文字符和完整英文单词，不统计标点与空白。
func ComputeWordChanges(previous, current string) WordChanges {
	if previous == current {
		return WordChanges{}
	}

	// 将计数单位映射为共享 rune，避免英文单词的局部修改被当成多个单词。
	units := make(map[string]rune)
	next := rune(1)
	encode := func(content string) []rune {
		var encoded []rune
		appendUnit := func(unit string) {
			id, ok := units[unit]
			if !ok {
				if next == 0xD800 {
					next = 0xE000
				}
				id = next
				next++
				units[unit] = id
			}
			encoded = append(encoded, id)
		}
		appendChinese := func(part string) {
			for _, r := range part {
				if unicode.Is(unicode.Han, r) {
					appendUnit(string(r))
				}
			}
		}
		start := 0
		for _, match := range enWordRe.FindAllStringIndex(content, -1) {
			appendChinese(content[start:match[0]])
			appendUnit(content[match[0]:match[1]])
			start = match[1]
		}
		appendChinese(content[start:])
		return encoded
	}
	before, after := encode(previous), encode(current)
	dmp := diffmatchpatch.New()
	dmp.DiffTimeout = 100 * time.Millisecond
	var changes WordChanges
	for _, diff := range dmp.DiffMainRunes(before, after, false) {
		switch diff.Type {
		case diffmatchpatch.DiffInsert:
			changes.Added += utf8.RuneCountInString(diff.Text)
		case diffmatchpatch.DiffDelete:
			changes.Deleted += utf8.RuneCountInString(diff.Text)
		}
	}
	return changes
}

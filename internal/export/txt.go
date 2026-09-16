package export

import (
	"fmt"
	"strings"

	"github.com/sigpanic/goink/internal/novel"
)

func exportTxt(n *novel.Novel, chapters []ChapterWithContent) ([]byte, string, error) {
	var b strings.Builder

	// 书名
	b.WriteString(n.Title)
	b.WriteString("\n\n")

	for _, cc := range chapters {
		ch := cc.Chapter
		title := ch.Title
		if title == "" {
			title = fmt.Sprintf("第%d章", ch.ChapterNumber)
		}

		fmt.Fprintf(&b, "第%d章 %s\n\n", ch.ChapterNumber, title)
		b.WriteString(strings.TrimSpace(cc.Content))
		b.WriteString("\n\n\n")
	}

	filename, err := DefaultFilename(n, "txt")
	if err != nil {
		return nil, "", err
	}
	return []byte(b.String()), filename, nil
}

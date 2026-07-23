package export

import (
	"fmt"
	"strings"
	"time"

	"github.com/sigpanic/goink/internal/novel"
)

func exportMarkdown(n *novel.Novel, chapters []ChapterWithContent) ([]byte, string, error) {
	var b strings.Builder

	// 书名 + 元信息
	fmt.Fprintf(&b, "# %s\n\n", n.Title)
	if n.Genre != "" {
		fmt.Fprintf(&b, "**类型**: %s  \n", n.Genre)
	}
	if n.Description != "" {
		fmt.Fprintf(&b, "**简介**: %s  \n", n.Description)
	}
	fmt.Fprintf(&b, "**导出时间**: %s  \n", time.Now().Format("2006-01-02 15:04"))
	b.WriteString("\n---\n\n")

	// 目录
	b.WriteString("## 目录\n\n")
	for _, cc := range chapters {
		ch := cc.Chapter
		title := ch.Title
		if title == "" {
			title = fmt.Sprintf("第%d章", ch.ChapterNumber)
		}
		fmt.Fprintf(&b, "- [第%d章 %s](#第%d章)\n\n", ch.ChapterNumber, title, ch.ChapterNumber)
	}
	b.WriteString("\n---\n\n")

	// 正文
	for _, cc := range chapters {
		ch := cc.Chapter
		title := ch.Title
		if title == "" {
			title = fmt.Sprintf("第%d章", ch.ChapterNumber)
		}
		fmt.Fprintf(&b, "## 第%d章 %s\n\n", ch.ChapterNumber, title)
		b.WriteString(cc.Content)
		b.WriteString("\n\n")
	}

	filename := safeFilename(n.Title) + ".md"
	return []byte(b.String()), filename, nil
}

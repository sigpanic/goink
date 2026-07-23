package export

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/novel"

	epub "github.com/bmaupin/go-epub"
	"github.com/yuin/goldmark"
)

func exportEpub(n *novel.Novel, chapters []ChapterWithContent, author string) ([]byte, string, error) {
	e := epub.NewEpub(n.Title)
	if author != "" {
		e.SetAuthor(author)
	} else {
		e.SetAuthor("Goink")
	}
	if n.Description != "" {
		e.SetDescription(n.Description)
	}

	// 尝试添加封面图片
	coverPath := filepath.Join(config.NovelDirPath(n.ID), git.CoverPath())
	if _, err := os.Stat(coverPath); err == nil {
		coverPathInEpub, err := e.AddImage(coverPath, git.CoverPath())
		if err == nil {
			e.SetCover(coverPathInEpub, "")
		}
	}

	md := goldmark.New()

	for _, cc := range chapters {
		ch := cc.Chapter
		var buf bytes.Buffer
		if err := md.Convert([]byte(cc.Content), &buf); err != nil {
			return nil, "", fmt.Errorf("epub: 第%d章 markdown 转换失败: %w", ch.ChapterNumber, err)
		}

		title := fmt.Sprintf("第%d章 %s", ch.ChapterNumber, ch.Title)
		sectionBody := fmt.Sprintf(`<html>
<head><style>%s</style></head>
<body><h1>%s</h1>
%s</body>
</html>`, epubCSS, title, buf.String())

		if _, err := e.AddSection(sectionBody, title, title, ""); err != nil {
			return nil, "", fmt.Errorf("epub: 添加章节失败: %w", err)
		}
	}

	var out bytes.Buffer
	if _, err := e.WriteTo(&out); err != nil {
		return nil, "", fmt.Errorf("epub: 写入失败: %w", err)
	}

	filename := safeFilename(n.Title) + ".epub"
	return out.Bytes(), filename, nil
}

var epubCSS = `
body { font-family: "Noto Serif SC", "Source Han Serif SC", serif; line-height: 1.8; margin: 1.5em; }
h1 { font-size: 1.6em; margin-bottom: 1em; text-align: center; }
p { text-indent: 2em; margin: 0.5em 0; }
`

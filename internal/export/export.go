package export

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/novel"
)

var safeFilenameRe = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

func safeFilename(s string) string {
	return safeFilenameRe.ReplaceAllString(strings.TrimSpace(s), "_")
}

// ChapterWithContent 将章节元数据和正文绑定在一起。
type ChapterWithContent struct {
	Chapter chapter.Chapter
	Content string
}

// ExportNovel 根据 format 生成导出内容，返回字节数据和默认文件名。
// author 优先作为 EPUB 元数据中的作者名，为空时使用默认值。
func ExportNovel(n *novel.Novel, chapters []ChapterWithContent, format, author string) ([]byte, string, error) {
	switch format {
	case "epub":
		return exportEpub(n, chapters, author)
	case "markdown":
		return exportMarkdown(n, chapters)
	case "txt":
		return exportTxt(n, chapters)
	default:
		return nil, "", fmt.Errorf("export: 不支持的格式: %s", format)
	}
}

// DefaultFilename 返回指定导出格式的默认文件名（与 ExportNovel 返回的一致）。
func DefaultFilename(n *novel.Novel, format string) (string, error) {
	ext, err := extension(format)
	if err != nil {
		return "", err
	}
	return safeFilename(n.Title) + ext, nil
}

func extension(format string) (string, error) {
	switch format {
	case "epub":
		return ".epub", nil
	case "markdown":
		return ".md", nil
	case "txt":
		return ".txt", nil
	default:
		return "", fmt.Errorf("export: 不支持的格式: %s", format)
	}
}

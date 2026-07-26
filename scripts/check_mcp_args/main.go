// check_mcp_args 检查所有 MCP 工具参数结构体（*Args / *Item）的 id 字段是否用 <entity>_id 命名，
// 而非裸 id。用法：go run ./scripts/check_mcp_args ./internal/mcp_tools/
//
// 背景：项目所有单实体 update/upsert 工具的参数一律用 <entity>_id（character_id / entry_id /
// setting_id / preference_id ...），这是与 LLM 的隐式约定——LLM 在 system prompt 引导下形成
// "更新某实体就传 <entity>_id"的稳定预期。裸 id 会破坏约定，且在 rawArgs patch 模式下
// （json.Unmarshal(tc.RawArgs, &entity)）覆盖 entity.ID，有数据安全隐患。
//
// 例外：delete_record 是通用删除工具，多表共用一个 id 字段，裸 id 合理。
// 在结构体上方加 //nolint:raw_id 注释可跳过检查。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// 匹配 *Args 或 *Item 结构体（MCP 工具的参数类型）
	reArgsStruct = regexp.MustCompile(`^type (\w+(Args|Item)) struct \{`)
	reJsonTag    = regexp.MustCompile(`json:"([^"]*)"`)
	reFieldName  = regexp.MustCompile(`^\s*(\w+)\s`)
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	violations := 0
	files := 0

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		files++

		violations += checkFile(path, string(content))
		return nil
	})

	if violations > 0 {
		fmt.Printf("\n%d field(s) use bare 'id' json tag. MCP params must use <entity>_id (e.g. setting_id, character_id).\n", violations)
		fmt.Println("Bare 'id' breaks the LLM convention and risks overwriting entity.ID in rawArgs patch.")
		fmt.Println("Add //nolint:raw_id comment above the struct to skip (only for generic tools like delete_record).")
		os.Exit(1)
	}
	fmt.Printf("OK: all MCP params use <entity>_id (checked %d files).\n", files)
}

// checkFile 检查单个文件，返回裸 id 字段数
func checkFile(path, content string) int {
	violations := 0
	lines := strings.Split(content, "\n")

	i := 0
	for i < len(lines) {
		line := lines[i]
		if !reArgsStruct.MatchString(line) {
			i++
			continue
		}

		structName := extractStructName(line)
		// 检查上方注释是否有 nolint:raw_id
		if hasNolintComment(lines, i) {
			i++
			continue
		}

		// 遍历字段
		i++
		for i < len(lines) {
			fieldLine := strings.TrimSpace(lines[i])
			if fieldLine == "}" {
				break
			}
			if fieldLine == "" || strings.HasPrefix(fieldLine, "//") {
				i++
				continue
			}
			if hasBareIDTag(fieldLine) {
				fieldName := extractFieldName(fieldLine)
				fmt.Printf("%s:%d: %s.%s uses bare 'id' json tag, should be <entity>_id\n",
					path, i+1, structName, fieldName)
				violations++
			}
			i++
		}
		i++
	}
	return violations
}

// hasNolintComment 检查结构体声明上方是否有 nolint:raw_id
func hasNolintComment(lines []string, typeLineIdx int) bool {
	for j := typeLineIdx - 1; j >= 0; j-- {
		line := strings.TrimSpace(lines[j])
		if line == "" {
			continue // 跳过空行
		}
		if strings.HasPrefix(line, "//") {
			if strings.Contains(line, "nolint:raw_id") {
				return true
			}
			continue // 继续往上找注释
		}
		break // 遇到非注释行，停止
	}
	return false
}

// hasBareIDTag 检查字段行是否用裸 id（json tag 第一段是 "id"）
func hasBareIDTag(line string) bool {
	matches := reJsonTag.FindStringSubmatch(line)
	if matches == nil {
		return false // 无 json tag，不检查
	}
	tagValue := matches[1]
	items := strings.Split(tagValue, ",")
	return items[0] == "id" // 裸 id（不含下划线前缀）
}

func extractStructName(line string) string {
	matches := reArgsStruct.FindStringSubmatch(line)
	if matches != nil {
		return matches[1]
	}
	return ""
}

func extractFieldName(line string) string {
	matches := reFieldName.FindStringSubmatch(line)
	if matches != nil {
		return matches[1]
	}
	return ""
}

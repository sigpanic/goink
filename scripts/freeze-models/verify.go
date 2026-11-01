package main

import (
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func verify(root, module string, source []byte, models []model) ([]byte, error) {
	// 原模型和冻结模型均使用历史版本的依赖建库，比较两者的数据库结构。
	// 这样可以独立验证源码转换，避免当前业务模型的变化影响校验结果。
	start := strings.Index(string(source), "\npackage ")
	end := strings.Index(string(source)[start+1:], "\n") + start + 1
	source = append(append([]byte(nil), source[:start]...), append([]byte("\npackage snapshot"), source[end:]...)...)
	dir := filepath.Join(root, "internal", "frozen_snapshot")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "models.go"), source, 0644); err != nil {
		return nil, err
	}
	var runner strings.Builder
	runner.WriteString("package main\nimport (\n\"encoding/json\"\n\"fmt\"\n\"os\"\n\"reflect\"\n\"gorm.io/gorm\"\n\"gorm.io/gorm/logger\"\n\"gorm.io/driver/sqlite\"\n")
	fmt.Fprintf(&runner, "snapshot %q\n", module+"/internal/frozen_snapshot")
	aliases := map[string]string{}
	for _, m := range models {
		if _, exists := aliases[m.path]; !exists {
			alias := fmt.Sprintf("model%d", len(aliases))
			aliases[m.path] = alias
			fmt.Fprintf(&runner, "%s %q\n", alias, m.path)
		}
	}
	runner.WriteString(")\nfunc originalModels() []any {return []any{\n")
	for _, m := range models {
		fmt.Fprintf(&runner, "&%s.%s{},\n", aliases[m.path], m.name)
	}
	runner.WriteString("}}\n")
	runner.WriteString(verifier)
	code, err := format.Source([]byte(runner.String()))
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, "freeze_verify.go")
	if err := os.WriteFile(path, code, 0644); err != nil {
		return nil, err
	}
	cmd := exec.Command("go", "run", "freeze_verify.go")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("verify frozen models: %w\n%s", err, out)
	}
	return out, nil
}

const verifier = `
type definition struct { Type, Name, SQL string }
func schema(models []any) ([]definition, error) {
    db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
    if err != nil { return nil, err }
    sqlDB, err := db.DB()
    if err != nil { return nil, err }
    defer sqlDB.Close()
    for _, model := range models { if err := db.AutoMigrate(model); err != nil { return nil, err } }
    rows, err := sqlDB.Query("SELECT type, name, sql FROM sqlite_master WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%' ORDER BY type, name")
    if err != nil { return nil, err }
    defer rows.Close()
    var result []definition
    for rows.Next() {
        var d definition
        if err := rows.Scan(&d.Type, &d.Name, &d.SQL); err != nil { return nil, err }
        result = append(result, d)
    }
    return result, rows.Err()
}
func main() {
    original, err := schema(originalModels())
    if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
    frozen, err := schema(snapshot.FrozenModels())
    if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
    if !reflect.DeepEqual(original, frozen) {
        fmt.Fprintf(os.Stderr, "schema differs: original=%d definitions, frozen=%d\n", len(original), len(frozen))
        for i, d := range original { if i >= len(frozen) || d != frozen[i] { fmt.Fprintf(os.Stderr, "original[%d]=%+v\n", i, d); if i < len(frozen) { fmt.Fprintf(os.Stderr, "frozen[%d]=%+v\n", i, frozen[i]) } } }
        os.Exit(1)
    }
    if err := json.NewEncoder(os.Stdout).Encode(frozen); err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
}
`

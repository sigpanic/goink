// freeze-models 从指定 Git 版本生成不可变的迁移模型快照。
// 在当前 worktree 根目录运行，例如：
// go run ./scripts/freeze-models -ref v1.6.0 -out internal/migrate/v160/models.gen.go
package main

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type sourceFile struct {
	file    *ast.File
	imports map[string]string
}

type sourceType struct {
	spec    *ast.TypeSpec
	file    *sourceFile
	methods []*ast.FuncDecl
	files   []*sourceFile
}

type model struct{ path, name string }

type freezer struct {
	root, module string
	fset         *token.FileSet
	packages     map[string]map[string]*sourceType
	emitted      map[model]bool
	imports      map[string]string
	declarations []string
}

func main() {
	ref := flag.String("ref", "HEAD", "Git revision containing the model definitions")
	out := flag.String("out", "", "output models.gen.go path inside this worktree")
	check := flag.Bool("check", false, "verify an existing snapshot without writing it")
	flag.Parse()
	if err := run(*ref, *out, *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ref, out string, check bool) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	gitRoot, err := git(root, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	if filepath.Clean(root) != filepath.Clean(gitRoot) {
		return fmt.Errorf("run from the worktree root")
	}
	if out == "" || filepath.Base(out) != "models.gen.go" {
		return fmt.Errorf("-out must name a models.gen.go file")
	}
	target, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("snapshot must be inside the current worktree")
	}
	pkg := filepath.Base(filepath.Dir(target))
	if !token.IsIdentifier(pkg) || token.Lookup(pkg).IsKeyword() {
		return fmt.Errorf("invalid snapshot package %q", pkg)
	}
	commit, err := git(root, "rev-parse", ref+"^{commit}")
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp(root, ".freeze-models-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	if err := extract(root, commit, work); err != nil {
		return err
	}
	mod, err := os.ReadFile(filepath.Join(work, "go.mod"))
	if err != nil {
		return err
	}
	fields := strings.Fields(string(mod))
	if len(fields) < 2 || fields[0] != "module" {
		return fmt.Errorf("invalid source go.mod")
	}
	f := &freezer{root: work, module: fields[1], fset: token.NewFileSet(), packages: map[string]map[string]*sourceType{}, emitted: map[model]bool{}, imports: map[string]string{}}
	models, err := f.models()
	if err != nil {
		return err
	}
	for _, m := range models {
		types, err := f.load(m.path)
		if err != nil {
			return err
		}
		found := false
		if t := types[m.name]; t != nil {
			for _, method := range t.methods {
				found = found || method.Name.Name == "TableName"
			}
		}
		if !found {
			return fmt.Errorf("model %s.%s needs an explicit TableName method", m.path, m.name)
		}
		if err := f.emit(m); err != nil {
			return err
		}
	}
	source, err := f.render(pkg, ref, commit, "", models)
	if err != nil {
		return err
	}
	schema, err := verify(work, f.module, source, models)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(schema)
	source, err = f.render(pkg, ref, commit, hex.EncodeToString(hash[:]), models)
	if err != nil {
		return err
	}
	if existing, err := os.ReadFile(target); err == nil {
		if !bytes.Equal(existing, source) {
			return fmt.Errorf("frozen snapshot %s differs; refusing to overwrite", out)
		}
	} else if !os.IsNotExist(err) {
		return err
	} else {
		if check {
			return fmt.Errorf("missing frozen snapshot %s", out)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		_, writeErr := file.Write(source)
		closeErr := file.Close()
		if writeErr != nil {
			_ = os.Remove(target)
			return writeErr
		}
		if closeErr != nil {
			_ = os.Remove(target)
			return closeErr
		}
	}
	fmt.Printf("Verified %d models from %s (%s), schema %x: %s\n", len(models), ref, commit, hash, out)
	return nil
}

func (f *freezer) parse(path string) (*sourceFile, error) {
	file, err := parser.ParseFile(f.fset, path, nil, 0)
	if err != nil {
		return nil, err
	}
	s := &sourceFile{file: file, imports: map[string]string{}}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, err
		}
		alias := filepath.Base(path)
		if imp.Name != nil {
			alias = imp.Name.Name
		}
		s.imports[alias] = path
	}
	return s, nil
}

func (f *freezer) models() ([]model, error) {
	// 历史版本在 Run 中声明局部 models 列表，新版本通过 Models() 返回清单。
	// 同时兼容这两种写法，避免扫描与持久化无关的 DTO 结构体。
	for _, name := range []string{"models.go", "migrate.go"} {
		path := filepath.Join(f.root, "internal", "migrate", name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		s, err := f.parse(path)
		if err != nil {
			return nil, err
		}
		var list *ast.CompositeLit
		ast.Inspect(s.file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.FuncDecl:
				if n.Name.Name == "Models" {
					ast.Inspect(n.Body, func(node ast.Node) bool {
						if ret, ok := node.(*ast.ReturnStmt); ok && len(ret.Results) == 1 {
							list, _ = ret.Results[0].(*ast.CompositeLit)
						}
						return true
					})
				}
			case *ast.AssignStmt:
				if len(n.Lhs) == 1 && len(n.Rhs) == 1 {
					if id, ok := n.Lhs[0].(*ast.Ident); ok && id.Name == "models" {
						list, _ = n.Rhs[0].(*ast.CompositeLit)
					}
				}
			}
			return true
		})
		if list == nil {
			continue
		}
		var models []model
		for _, entry := range list.Elts {
			pointer, ok := entry.(*ast.UnaryExpr)
			if !ok || pointer.Op != token.AND {
				return nil, fmt.Errorf("unsupported model expression in %s", path)
			}
			literal, ok := pointer.X.(*ast.CompositeLit)
			if !ok {
				return nil, fmt.Errorf("unsupported model value in %s", path)
			}
			selector, ok := literal.Type.(*ast.SelectorExpr)
			if !ok {
				return nil, fmt.Errorf("model must be a qualified type in %s", path)
			}
			alias, ok := selector.X.(*ast.Ident)
			if !ok || s.imports[alias.Name] == "" {
				return nil, fmt.Errorf("unknown model import in %s", path)
			}
			m := model{s.imports[alias.Name], selector.Sel.Name}
			if m.path == f.module+"/internal/migrate/engine" && m.name == "MigrateState" {
				continue
			}
			models = append(models, m)
		}
		if len(models) == 0 {
			return nil, fmt.Errorf("empty source model list")
		}
		return models, nil
	}
	return nil, fmt.Errorf("cannot locate source migration models")
}

func (f *freezer) load(path string) (map[string]*sourceType, error) {
	if types, ok := f.packages[path]; ok {
		return types, nil
	}
	if !strings.HasPrefix(path, f.module+"/internal/") {
		return nil, fmt.Errorf("unsupported source package %s", path)
	}
	entries, err := os.ReadDir(filepath.Join(f.root, filepath.FromSlash(strings.TrimPrefix(path, f.module+"/"))))
	if err != nil {
		return nil, err
	}
	types := map[string]*sourceType{}
	var files []*sourceFile
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		s, err := f.parse(filepath.Join(f.root, filepath.FromSlash(strings.TrimPrefix(path, f.module+"/")), entry.Name()))
		if err != nil {
			return nil, err
		}
		files = append(files, s)
		for _, decl := range s.file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				t := spec.(*ast.TypeSpec)
				if _, duplicate := types[t.Name.Name]; duplicate {
					return nil, fmt.Errorf("ambiguous source type %s.%s", path, t.Name.Name)
				}
				types[t.Name.Name] = &sourceType{spec: t, file: s}
			}
		}
	}
	for _, s := range files {
		for _, decl := range s.file.Decls {
			method, ok := decl.(*ast.FuncDecl)
			if !ok || method.Recv == nil {
				continue
			}
			switch method.Name.Name {
			case "TableName", "Scan", "Value", "GormDataType", "GormDBDataType":
			default:
				continue
			}
			receiver := method.Recv.List[0].Type
			if star, ok := receiver.(*ast.StarExpr); ok {
				receiver = star.X
			}
			id, ok := receiver.(*ast.Ident)
			if !ok || types[id.Name] == nil {
				return nil, fmt.Errorf("unsupported schema method receiver in %s", path)
			}
			t := types[id.Name]
			t.methods, t.files = append(t.methods, method), append(t.files, s)
		}
	}
	f.packages[path] = types
	return types, nil
}

func frozenName(m model) string {
	parts := strings.Split(strings.TrimPrefix(m.path, strings.Split(m.path, "/internal/")[0]+"/internal/"), "/")
	name := "Frozen"
	for _, part := range parts {
		name += strings.ToUpper(part[:1]) + part[1:]
	}
	return name + m.name
}

func (f *freezer) emit(m model) error {
	if f.emitted[m] {
		return nil
	}
	types, err := f.load(m.path)
	if err != nil {
		return err
	}
	t := types[m.name]
	if t == nil {
		return fmt.Errorf("missing source type %s.%s", m.path, m.name)
	}
	f.emitted[m] = true
	if err := f.rewrite(t.spec.Type, m.path, t.file); err != nil {
		return err
	}
	t.spec.Name.Name = frozenName(m)
	var body bytes.Buffer
	if err := format.Node(&body, f.fset, &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{t.spec}}); err != nil {
		return err
	}
	f.declarations = append(f.declarations, body.String())
	for i, method := range t.methods {
		if err := f.rewrite(method, m.path, t.files[i]); err != nil {
			return err
		}
		body.Reset()
		if err := format.Node(&body, f.fset, method); err != nil {
			return err
		}
		f.declarations = append(f.declarations, body.String())
	}
	return nil
}

func (f *freezer) rewrite(node ast.Node, path string, file *sourceFile) error {
	types, err := f.load(path)
	if err != nil {
		return err
	}
	var rewriteErr error
	ast.Inspect(node, func(n ast.Node) bool {
		if rewriteErr != nil {
			return false
		}
		if field, ok := n.(*ast.Field); ok {
			if selector, ok := field.Type.(*ast.SelectorExpr); ok {
				if id, ok := selector.X.(*ast.Ident); ok {
					if imported := file.imports[id.Name]; strings.HasPrefix(imported, f.module+"/") {
						m := model{imported, selector.Sel.Name}
						rewriteErr = f.emit(m)
						if rewriteErr == nil {
							field.Type = ast.NewIdent(frozenName(m))
						}
						return false
					}
				}
			}
			rewriteErr = f.rewrite(field.Type, path, file)
			return false
		}
		if selector, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := selector.X.(*ast.Ident); ok {
				if imported := file.imports[id.Name]; imported != "" {
					if strings.HasPrefix(imported, f.module+"/") {
						m := model{imported, selector.Sel.Name}
						rewriteErr = f.emit(m)
						// 跨包类型需要替换 AST 节点；仅清空包限定名会生成无效 Go 代码。
						// 尚未支持这种依赖时明确报错，避免生成不完整的模型快照。
						if rewriteErr == nil {
							rewriteErr = fmt.Errorf("cross-package field type %s requires explicit support", imported+"."+m.name)
						}
					} else {
						if other := f.imports[id.Name]; other != "" && other != imported {
							rewriteErr = fmt.Errorf("conflicting schema import alias %s", id.Name)
						} else {
							f.imports[id.Name] = imported
						}
					}
					return false
				}
			}
		}
		if id, ok := n.(*ast.Ident); ok && types[id.Name] != nil {
			m := model{path, id.Name}
			rewriteErr = f.emit(m)
			id.Name = frozenName(m)
		}
		return true
	})
	return rewriteErr
}

func (f *freezer) render(pkg, ref, commit, hash string, models []model) ([]byte, error) {
	var source strings.Builder
	fmt.Fprintf(&source, "// Code generated by freeze-models; DO NOT EDIT.\n// Source: %s\n// Commit: %s\n// Schema SHA256: %s\n\npackage %s\n\n", ref, commit, hash, pkg)
	if len(f.imports) > 0 {
		source.WriteString("import (\n")
		var aliases []string
		for alias := range f.imports {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		for _, alias := range aliases {
			fmt.Fprintf(&source, "%s %q\n", alias, f.imports[alias])
		}
		source.WriteString(")\n\n")
	}
	for _, declaration := range f.declarations {
		source.WriteString(declaration + "\n\n")
	}
	source.WriteString("func FrozenModels() []any {\nreturn []any{\n")
	for _, m := range models {
		fmt.Fprintf(&source, "&%s{},\n", frozenName(m))
	}
	source.WriteString("}\n}\n")
	return format.Source([]byte(source.String()))
}

func git(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %v: %w: %s", args, err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

func extract(root, commit, dir string) error {
	cmd := exec.Command("git", "archive", commit)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	reader, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	extractErr := extractTar(reader, dir)
	if extractErr != nil {
		_ = cmd.Process.Kill()
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("archive source: %w: %s", err, stderr.String())
	}
	return extractErr
}

func extractTar(reader io.Reader, dir string) error {
	tr := tar.NewReader(reader)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if header.Name != "go.mod" && header.Name != "go.sum" && !strings.HasPrefix(header.Name, "internal/") && !strings.HasPrefix(header.Name, "third_party/") {
			continue
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		name := filepath.Clean(filepath.FromSlash(header.Name))
		if header.Typeflag != tar.TypeReg || filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unsupported source archive entry %s", header.Name)
		}
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(header.Mode))
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(file, tr)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
}
